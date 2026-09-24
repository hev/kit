package store

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/minio/minio-go/v7"
)

type Store struct {
	client *minio.Client
	bucket string
}

// OTLPSession aggregates per-session OTLP records into a single row.
type OTLPSession struct {
	ID                  string
	Date                string
	FirstTS             time.Time
	LastTS              time.Time
	EventCount          int
	PromptCount         int
	FirstPrompt         string
	Service             string
	Harness             string
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	CostUSD             float64
	// Model is the primary model used in the session (e.g. "claude-opus-4-7"),
	// preferring the largest/most-used model when more than one shows up.
	Model string
	// GHUser is the captured GitHub login of whoever produced the session, if known.
	GHUser string
	// TitleRef is the body_ref for the Haiku-generated session title body.
	// Resolve to a human-readable string by calling LoadTitles.
	TitleRef string
	// Title is the resolved session title; populated by LoadTitles, empty otherwise.
	Title string
}

// OTLPEvent is a single decoded OTLP log record from Fluent Bit.
type OTLPEvent struct {
	Timestamp time.Time
	Name      string
	SessionID string
	Attrs     map[string]any
}

func New(cfg *daemon.Config) (*Store, error) {
	u, err := url.Parse(cfg.S3Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}
	host := u.Host
	if host == "" {
		host = strings.TrimPrefix(strings.TrimPrefix(cfg.S3Endpoint, "http://"), "https://")
	}
	useSSL := u.Scheme == "https"

	client, err := minio.New(host, &minio.Options{
		Creds:  daemon.S3Credentials(cfg),
		Secure: useSSL,
		Region: cfg.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &Store{client: client, bucket: cfg.S3Bucket}, nil
}

func (s *Store) ListDates(ctx context.Context) ([]string, error) {
	var dates []string
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    "raw/dt=",
		Recursive: false,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list dates: %w", obj.Err)
		}
		// Prefix looks like "raw/dt=2026-05-12/"
		p := strings.TrimPrefix(obj.Key, "raw/dt=")
		p = strings.TrimSuffix(p, "/")
		if p != "" {
			dates = append(dates, p)
		}
	}
	return dates, nil
}

func (s *Store) fetchObject(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

func truncate(s string, max int) string {
	// Collapse newlines to spaces for display.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// ListOTLPSessions scans otlp/dt=<date>/ jsonl objects and aggregates by session.id.
func (s *Store) ListOTLPSessions(ctx context.Context, date string) ([]OTLPSession, error) {
	prefix := fmt.Sprintf("otlp/dt=%s/", date)
	sessions := map[string]*OTLPSession{}

	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list otlp: %w", obj.Err)
		}
		if !strings.HasSuffix(obj.Key, ".jsonl") {
			continue
		}
		if err := s.foldOTLPObject(ctx, obj.Key, date, sessions); err != nil {
			return nil, err
		}
	}

	out := make([]OTLPSession, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, *sess)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FirstTS.Before(out[j].FirstTS)
	})
	return out, nil
}

func (s *Store) foldOTLPObject(ctx context.Context, key, date string, sessions map[string]*OTLPSession) error {
	data, err := s.fetchObject(ctx, key)
	if err != nil {
		return nil // best-effort; skip unreadable
	}
	// per-session counts of which model was requested, so we can pick the
	// "primary" one (api_request events from Haiku are title-gen noise).
	modelCounts := map[string]map[string]int{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		evt, ok := decodeOTLPLine(scanner.Bytes())
		if !ok || evt.SessionID == "" {
			continue
		}
		sess := sessions[evt.SessionID]
		if sess == nil {
			sess = &OTLPSession{ID: evt.SessionID, Date: date, FirstTS: evt.Timestamp, LastTS: evt.Timestamp, Harness: "claude_code"}
			sessions[evt.SessionID] = sess
		}
		sess.EventCount++
		if !evt.Timestamp.IsZero() {
			if sess.FirstTS.IsZero() || evt.Timestamp.Before(sess.FirstTS) {
				sess.FirstTS = evt.Timestamp
			}
			if evt.Timestamp.After(sess.LastTS) {
				sess.LastTS = evt.Timestamp
			}
		}
		switch evt.Name {
		case "user_prompt":
			sess.PromptCount++
			if sess.FirstPrompt == "" {
				if p, ok := evt.Attrs["prompt"].(string); ok {
					sess.FirstPrompt = truncate(p, 60)
				}
			}
		case "api_request":
			sess.InputTokens += attrInt(evt.Attrs, "input_tokens")
			sess.OutputTokens += attrInt(evt.Attrs, "output_tokens")
			sess.CacheReadTokens += attrInt(evt.Attrs, "cache_read_tokens")
			sess.CacheCreationTokens += attrInt(evt.Attrs, "cache_creation_tokens")
			sess.CostUSD += attrFloat(evt.Attrs, "cost_usd")
			if model, _ := evt.Attrs["model"].(string); model != "" {
				m := modelCounts[evt.SessionID]
				if m == nil {
					m = map[string]int{}
					modelCounts[evt.SessionID] = m
				}
				m[model]++
			}
		case "api_response_body":
			if sess.TitleRef == "" {
				if qs, _ := evt.Attrs["query_source"].(string); qs == "generate_session_title" {
					if ref, _ := evt.Attrs["body_ref"].(string); ref != "" {
						sess.TitleRef = ref
					}
				}
			}
		}
		if sess.Service == "" {
			if svc, ok := evt.Attrs["__service__"].(string); ok {
				sess.Service = svc
			}
		}
	}
	for sid, counts := range modelCounts {
		if sess := sessions[sid]; sess != nil && sess.Model == "" {
			sess.Model = pickPrimaryModel(counts)
		}
	}
	return nil
}

// pickPrimaryModel chooses the "main" model for a session, ignoring Haiku when
// other models are present (Haiku is used internally for title generation).
func pickPrimaryModel(counts map[string]int) string {
	var best string
	var bestCount int
	for model, n := range counts {
		if best == "" {
			best, bestCount = model, n
			continue
		}
		bestHaiku := strings.Contains(strings.ToLower(best), "haiku")
		curHaiku := strings.Contains(strings.ToLower(model), "haiku")
		if bestHaiku && !curHaiku {
			best, bestCount = model, n
			continue
		}
		if !bestHaiku && curHaiku {
			continue
		}
		if n > bestCount {
			best, bestCount = model, n
		}
	}
	return best
}

// GetOTLPSession returns the matched session and its events (sorted by ts) for any
// session whose ID has the given prefix on the given date.
func (s *Store) GetOTLPSession(ctx context.Context, date, idPrefix string) (OTLPSession, []OTLPEvent, error) {
	prefix := fmt.Sprintf("otlp/dt=%s/", date)
	var (
		out          []OTLPEvent
		matched      string
		earliest     time.Time
		latest       time.Time
		service      string
		prompts      int
		first        string
		inTok        int64
		outTok       int64
		cacheReadTok int64
		cacheCrTok   int64
		cost         float64
		titleRef     string
		modelCounts  = map[string]int{}
	)

	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return OTLPSession{}, nil, fmt.Errorf("list otlp: %w", obj.Err)
		}
		if !strings.HasSuffix(obj.Key, ".jsonl") {
			continue
		}
		data, err := s.fetchObject(ctx, obj.Key)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			evt, ok := decodeOTLPLine(scanner.Bytes())
			if !ok || evt.SessionID == "" {
				continue
			}
			if !strings.HasPrefix(evt.SessionID, idPrefix) {
				continue
			}
			if matched == "" {
				matched = evt.SessionID
			} else if matched != evt.SessionID {
				return OTLPSession{}, nil, fmt.Errorf("prefix %q matches multiple sessions (%s, %s)", idPrefix, matched, evt.SessionID)
			}
			out = append(out, evt)
			if !evt.Timestamp.IsZero() {
				if earliest.IsZero() || evt.Timestamp.Before(earliest) {
					earliest = evt.Timestamp
				}
				if evt.Timestamp.After(latest) {
					latest = evt.Timestamp
				}
			}
			switch evt.Name {
			case "user_prompt":
				prompts++
				if first == "" {
					if p, ok := evt.Attrs["prompt"].(string); ok {
						first = truncate(p, 60)
					}
				}
			case "api_request":
				inTok += attrInt(evt.Attrs, "input_tokens")
				outTok += attrInt(evt.Attrs, "output_tokens")
				cacheReadTok += attrInt(evt.Attrs, "cache_read_tokens")
				cacheCrTok += attrInt(evt.Attrs, "cache_creation_tokens")
				cost += attrFloat(evt.Attrs, "cost_usd")
				if model, _ := evt.Attrs["model"].(string); model != "" {
					modelCounts[model]++
				}
			case "api_response_body":
				if titleRef == "" {
					if qs, _ := evt.Attrs["query_source"].(string); qs == "generate_session_title" {
						if ref, _ := evt.Attrs["body_ref"].(string); ref != "" {
							titleRef = ref
						}
					}
				}
			}
			if service == "" {
				if svc, ok := evt.Attrs["__service__"].(string); ok {
					service = svc
				}
			}
		}
	}

	if matched == "" {
		return OTLPSession{}, nil, fmt.Errorf("no OTLP session matching %q on %s", idPrefix, date)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})

	return OTLPSession{
		ID:                  matched,
		Date:                date,
		FirstTS:             earliest,
		LastTS:              latest,
		EventCount:          len(out),
		PromptCount:         prompts,
		FirstPrompt:         first,
		Service:             service,
		Harness:             "claude_code",
		InputTokens:         inTok,
		OutputTokens:        outTok,
		CacheReadTokens:     cacheReadTok,
		CacheCreationTokens: cacheCrTok,
		CostUSD:             cost,
		Model:               pickPrimaryModel(modelCounts),
		TitleRef:            titleRef,
	}, out, nil
}

// LoadTitles resolves each session's TitleRef by fetching the generate_session_title
// response body from object storage and parsing out the model's reply. Sessions
// with no TitleRef or already-resolved Title are skipped. Best-effort: fetch
// errors leave Title empty.
func (s *Store) LoadTitles(ctx context.Context, sessions []OTLPSession) {
	const maxConcurrent = 8
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i := range sessions {
		if sessions[i].TitleRef == "" || sessions[i].Title != "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			if t, err := s.fetchTitle(ctx, sessions[idx].Date, sessions[idx].TitleRef); err == nil {
				sessions[idx].Title = t
			}
		}(i)
	}
	wg.Wait()
}

// fetchTitle loads a title-generation response envelope from object storage and
// extracts the title string. The model's reply is typically a tiny JSON like
// {"title":"..."}, but we fall back to the raw text if that doesn't parse.
func (s *Store) fetchTitle(ctx context.Context, date, ref string) (string, error) {
	body, err := s.GetRawAPIBody(ctx, date, ref)
	if err != nil {
		return "", err
	}
	var env daemon.TraceEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", err
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(env.PayloadRaw), &resp); err != nil {
		return "", err
	}
	for _, c := range resp.Content {
		if c.Type != "text" {
			continue
		}
		text := strings.TrimSpace(c.Text)
		var j struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(text), &j); err == nil && j.Title != "" {
			return j.Title, nil
		}
		if text != "" {
			return text, nil
		}
	}
	return "", fmt.Errorf("no title in response")
}

// GetRawAPIBody fetches the wrapped envelope for a raw API request/response body
// by its local-filesystem basename (e.g. "req_….response.json").
func (s *Store) GetRawAPIBody(ctx context.Context, date, basename string) (json.RawMessage, error) {
	base := path.Base(basename)
	key := fmt.Sprintf("raw/dt=%s/source=claude-raw-api-bodies/%s", date, base)
	return s.fetchObject(ctx, key)
}

func attrInt(a map[string]any, key string) int64 {
	switch v := a[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func attrFloat(a map[string]any, key string) float64 {
	switch v := a[key].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

// decodeOTLPLine parses one record produced by Fluent Bit's opentelemetry input.
func decodeOTLPLine(line []byte) (OTLPEvent, bool) {
	if len(bytes.TrimSpace(line)) == 0 {
		return OTLPEvent{}, false
	}
	var raw struct {
		Internal struct {
			GroupAttributes struct {
				Resource struct {
					Attributes map[string]any `json:"attributes"`
				} `json:"resource"`
			} `json:"group_attributes"`
			LogMetadata struct {
				OTLP struct {
					Timestamp  int64          `json:"timestamp"`
					Attributes map[string]any `json:"attributes"`
				} `json:"otlp"`
			} `json:"log_metadata"`
		} `json:"__internal__"`
		Log string `json:"log"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return OTLPEvent{}, false
	}

	attrs := raw.Internal.LogMetadata.OTLP.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	if svc, ok := raw.Internal.GroupAttributes.Resource.Attributes["service.name"].(string); ok {
		attrs["__service__"] = svc
	}

	evt := OTLPEvent{Attrs: attrs}
	if ts := raw.Internal.LogMetadata.OTLP.Timestamp; ts > 0 {
		evt.Timestamp = time.Unix(0, ts).UTC()
	}
	if name, ok := attrs["event.name"].(string); ok {
		evt.Name = name
	} else if raw.Log != "" {
		evt.Name = strings.TrimPrefix(raw.Log, "claude_code.")
	}
	if sid, ok := attrs["session.id"].(string); ok {
		evt.SessionID = sid
	}
	return evt, true
}
