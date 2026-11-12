package dispatch

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

// A traced run with a context recipe carries both, through the real
// admission, bundle build and supervisor launch: the run record, the attempt
// result, the worker's environment and standard input, and the per-session
// attribution the indexer copies onto the session row.
func TestRunCarriesTraceAndContext(t *testing.T) {
	clearTraceEnv(t)
	h := newContextHarness(t)
	text := contextOrder("traced-ctx", "claude", "recipes/ok.json")
	o, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	m := Meta{Order: o}
	if err := StampTrace(&m, TraceStart{Caller: Caller{Session: "foh-test", Harness: "claude", Host: "laptop"}, Text: OrderAskText(text)}); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(Request{Action: "start", Name: o.Name, Order: text, Meta: m, Workspace: h.workspace}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "traced-ctx")
	m = runMeta(t, "traced-ctx")
	if !ValidTraceID(m.TraceID) || !ValidSpanID(m.SpanID) || RootAsk(m) == nil || m.ParentSpanID != RootAsk(m).SpanID {
		t.Fatalf("trace not recorded on the run: %+v", m)
	}
	if m.ContextBundle == nil || m.ContextDigest == "" || m.ContextBundle.Digest != m.ContextDigest {
		t.Fatalf("context not recorded on the run: %+v", m.ContextBundle)
	}
	if err := superviseRound("traced-ctx"); err != nil {
		t.Fatal(err)
	}

	// The worker got the bundle on standard input and the attempt's trace in
	// its environment.
	stdin, _ := h.captured(1, "claude")
	contextMD, _ := os.ReadFile(filepath.Join(dir, "contexts", "1", "context.md"))
	if len(stdin) == 0 || !bytes.Equal(stdin, contextMD) {
		t.Fatalf("stdin %q", stdin)
	}
	env, _ := os.ReadFile(filepath.Join(h.capture, "1-claude.env"))
	want := Traceparent(m.TraceID, m.SpanID) + "\n" + m.TraceID + "\n" + m.SpanID
	if string(env) != want {
		t.Fatalf("worker trace env %q, want %q", env, want)
	}

	// The attempt result keeps both, so an archived attempt still joins.
	r := ctxResult(t, dir)
	if r.Session != "claude-1" || r.TraceID != m.TraceID || r.SpanID != m.SpanID || r.ContextDigest != m.ContextDigest || r.Error != "" {
		t.Fatalf("result %+v", r)
	}
	// What the indexer writes on the worker's session row.
	wt := filepath.Join(dir, "wt")
	if trace, span := TraceAttribution(wt, "claude-1"); trace != m.TraceID || span != m.SpanID {
		t.Fatalf("trace attribution %s %s", trace, span)
	}
	if got := ContextAttribution(wt, "claude-1"); got != m.ContextDigest {
		t.Fatalf("context attribution %s", got)
	}

	// The attempt's published trace facts (the Asks view's span row) carry
	// the trace, the context digest and recipe, and the requester together.
	var span *trace.SpanRow
	for _, row := range TraceSpanRows(ReadRunRecords(Root()), "here", nil) {
		if row.Kind == "attempt" && row.Run == "traced-ctx" {
			row := row
			span = &row
		}
	}
	if span == nil || span.TraceID != m.TraceID || span.ID != m.SpanID || span.ContextDigest != m.ContextDigest ||
		span.ContextRecipe != "recipes/ok.json" || span.RequestedBy != requester(m) {
		t.Fatalf("attempt span %+v", span)
	}

	// show lists both.
	show, err := Handle(Request{Action: "show", Name: "traced-ctx"})
	if err != nil || !strings.Contains(show.Files["meta.json"], m.TraceID) || !strings.Contains(show.Files["meta.json"], m.ContextDigest) {
		t.Fatalf("show %v %s", err, show.Files["meta.json"])
	}
}
