package collect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/redact"
	"github.com/oklog/ulid/v2"
)

// RedactionPolicy names kit's default capture policy on an envelope.
const RedactionPolicy = "basic"

// Redact scrubs every string a span carries (attribute values, nested in
// arrays and maps, the status message and resource attributes) in place, and
// returns the number of replacements. A nil scrubber is capture.redact=false.
func Redact(s *redact.Scrubber, sp *Span) int {
	if s == nil {
		return 0
	}
	n := 0
	text := func(v string) string {
		out, counts := s.Text(v)
		for _, c := range counts {
			n += c
		}
		return out
	}
	var walk func(v any) any
	walk = func(v any) any {
		switch t := v.(type) {
		case string:
			return text(t)
		case []any:
			for i := range t {
				t[i] = walk(t[i])
			}
			return t
		case map[string]any:
			for k := range t {
				t[k] = walk(t[k])
			}
			return t
		}
		return v
	}
	for k := range sp.Attrs {
		sp.Attrs[k] = walk(sp.Attrs[k])
	}
	sp.StatusMessage = text(sp.StatusMessage)
	defer func() { sp.Redactions += n }()
	if sp.Resource != nil {
		// A batch's spans share one resource map; scrub a copy once per span
		// so the count stays per span and the shared map is not rewritten
		// under another span's feet.
		res := make(map[string]any, len(sp.Resource))
		for k, v := range sp.Resource {
			res[k] = walk(copyValue(v))
		}
		sp.Resource = res
	}
	return n
}

func copyValue(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = copyValue(t[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = copyValue(e)
		}
		return out
	}
	return v
}

// spanPayload is payload_raw: the span as app capture read it, after
// redaction.
type spanPayload struct {
	TraceID       string         `json:"trace_id"`
	SpanID        string         `json:"span_id"`
	ParentSpanID  string         `json:"parent_span_id,omitempty"`
	Name          string         `json:"name"`
	Scope         string         `json:"scope,omitempty"`
	Start         string         `json:"start"`
	End           string         `json:"end,omitempty"`
	StatusCode    int            `json:"status_code,omitempty"`
	StatusMessage string         `json:"status_message,omitempty"`
	Attributes    map[string]any `json:"attributes"`
}

// Envelope wraps one span. harness is app:<service.name>; the turn's owner and
// tenant ride in resource as end_user and tenant beside the OTel resource
// attributes.
func Envelope(a Attributed, host string, policy string) *daemon.TraceEnvelope {
	sp := a.Span
	service := firstNonEmpty(attrString(sp.Resource, attrService), "unknown")
	resource := map[string]any{}
	for k, v := range sp.Resource {
		resource[k] = v
	}
	if a.Owner != "" {
		resource["end_user"] = a.Owner
	}
	if a.Tenant != "" {
		resource["tenant"] = a.Tenant
	}
	if a.ChatID != "" {
		resource["chat_id"] = a.ChatID
	}
	resJSON, _ := json.Marshal(resource)
	res := string(resJSON)
	payload, _ := json.Marshal(spanPayload{
		TraceID: sp.TraceID, SpanID: sp.SpanID, ParentSpanID: sp.ParentSpanID, Name: sp.Name, Scope: sp.Scope,
		Start: ts(sp.Start), End: ts(sp.End), StatusCode: sp.StatusCode, StatusMessage: sp.StatusMessage, Attributes: sp.Attrs,
	})
	source := sp.Start
	if source.IsZero() {
		source = time.Now()
	}
	env := &daemon.TraceEnvelope{
		IngestID:           ulid.Make().String(),
		IngestTS:           time.Now().UTC().Format(time.RFC3339),
		IngestSource:       "otlp_http",
		IngestAgent:        "hev-kit",
		IngestAgentVersion: daemon.IngestAgentVersion(),
		SourceTS:           source.UTC().Format(time.RFC3339Nano),
		Host:               host,
		Harness:            "app:" + service,
		SessionID:          firstNonEmpty(a.ChatID, sp.TraceID),
		TurnID:             a.TurnID,
		EventKind:          sp.Name,
		SignalType:         "trace",
		PayloadRaw:         string(payload),
		PayloadSchemaRef:   "otel.span.ai-sdk.v1",
		Resource:           &res,
	}
	if policy != "" {
		applied := sp.Redactions > 0
		count := sp.Redactions
		env.RedactionPolicy, env.RedactionApplied, env.RedactionCount = policy, &applied, &count
	}
	return env
}

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// EnvelopeSink receives envelopes.
type EnvelopeSink interface {
	Write(envs []*daemon.TraceEnvelope) error
}

// FileSink appends envelopes as JSON lines under Dir, partitioned the way the
// raw archive is: dt=YYYY-MM-DD/harness=<harness>/envelopes.jsonl.
type FileSink struct {
	Dir string
	mu  sync.Mutex
}

func (f *FileSink) Write(envs []*daemon.TraceEnvelope) error {
	if len(envs) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	byPath := map[string][]*daemon.TraceEnvelope{}
	for _, e := range envs {
		t, err := time.Parse(time.RFC3339Nano, e.SourceTS)
		if err != nil {
			t = time.Now()
		}
		harness := strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(e.Harness)
		p := filepath.Join(f.Dir, "dt="+daemon.PartitionDateFromTime(t), "harness="+harness, "envelopes.jsonl")
		byPath[p] = append(byPath[p], e)
	}
	for p, es := range byPath {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		fh, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		w := bufio.NewWriter(fh)
		enc := json.NewEncoder(w)
		for _, e := range es {
			if err := enc.Encode(e); err != nil {
				fh.Close()
				return err
			}
		}
		if err := w.Flush(); err != nil {
			fh.Close()
			return err
		}
		if err := fh.Close(); err != nil {
			return fmt.Errorf("envelopes %s: %w", p, err)
		}
	}
	return nil
}
