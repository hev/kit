package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/tracefixture"
)

// archive collects what each host's indexer publishes, standing in for the
// shared spans namespace.
type archive struct{ rows map[string]trace.SpanRow }

func (a *archive) WriteTraceSpans(rows []trace.SpanRow) (layer.WriteResult, error) {
	for _, r := range rows {
		a.rows[r.ID] = r
	}
	return layer.WriteResult{RowsUpserted: len(rows)}, nil
}

func (a *archive) all() []trace.SpanRow {
	var out []trace.SpanRow
	for _, r := range a.rows {
		out = append(out, r)
	}
	return out
}

// Run records on one host, the dashboard on another, both pointed at the
// same archive: the dashboard shows the other host's traces, whole.
func TestAsksAcrossHostsThroughTheArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	info, err := tracefixture.Write(home, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sessions, blocks, err := index.ReadSide(&trace.ClaudeSource{Root: info.Transcripts}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	// Oven holds the implementation runs; the mini holds the browser
	// validation run it executed.
	oven := filepath.Join(home, "dispatch")
	mini := filepath.Join(t.TempDir(), "dispatch")
	if err := os.MkdirAll(mini, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(oven, tracefixture.Validation), filepath.Join(mini, tracefixture.Validation)); err != nil {
		t.Fatal(err)
	}

	shared := &archive{rows: map[string]trace.SpanRow{}}
	for _, root := range []string{oven, mini} {
		st := &index.State{Units: map[string]string{}}
		rep, err := index.RunTraceSpans(root, shared, st, false)
		if err != nil || len(rep.Errors) > 0 {
			t.Fatal(err, rep.Errors)
		}
		// A second publish with nothing changed writes nothing.
		again, _ := index.RunTraceSpans(root, shared, st, false)
		if again.UnitsIndexed != 0 || again.UnitsSkipped != rep.UnitsSeen {
			t.Fatalf("republish: %+v", again)
		}
	}

	// Only what the view shows leaves the host: no order body beyond the
	// title line, no Acceptance, no report.
	b, _ := json.Marshal(shared.all())
	for _, private := range []string{"Group the guides by task", "Existing guide URLs still resolve", "Regrouped the sidebar", "## Acceptance", "Verdict: pass"} {
		if strings.Contains(string(b), private) {
			t.Fatalf("published %q", private)
		}
	}

	read := func(root string, spans []trace.SpanRow) (TraceSummary, []Span, []TraceSummary) {
		h := New(&MemoryReader{Sessions: sessions, Blocks: blocks, Spans: spans}).WithDispatchRoot(root).Handler()
		var list struct {
			Traces []TraceSummary `json:"traces"`
		}
		getAPI(t, h, "/api/asks?window=all", &list)
		var detail struct {
			Trace TraceSummary `json:"trace"`
			Spans []Span       `json:"spans"`
		}
		for _, tr := range list.Traces {
			if tr.ID == info.Trace {
				getAPI(t, h, "/api/asks/"+info.Trace, &detail)
			}
		}
		return detail.Trace, detail.Spans, list.Traces
	}
	kinds := func(spans []Span) map[string]int {
		k := map[string]int{}
		for _, s := range spans {
			k[s.Kind]++
		}
		return k
	}

	// Without an archive each dashboard sees only its own host's runs.
	if _, _, list := read(mini, nil); len(list) != 1 {
		t.Fatalf("mini alone: %+v", list)
	}

	// The mini's dashboard: Oven's runs come from the archive, the mini's
	// validation run from disk, and they read as one trace.
	tr, spans, list := read(mini, shared.all())
	if len(list) != 3 {
		t.Fatalf("mini sees %d traces: %+v", len(list), list)
	}
	if k := kinds(spans); tr.ID != info.Trace || len(tr.Orders) != 2 || tr.Asks != 4 || tr.Runs != 4 || tr.Sessions != 4 || k["run"] != 3 || k["validation"] != 1 || k["order"] != 2 || k["session"] != 4 || !tr.RootHere {
		t.Fatalf("mini trace: %+v %v", tr, k)
	}
	for _, s := range spans {
		switch {
		case s.Kind == "run" && (s.Remote == "" || s.Report != "" || s.SessionID == "" || s.State != "done"):
			t.Fatalf("remote attempt: %+v", s)
		case s.Kind == "validation" && (s.Remote != "" || s.State != "pass"):
			t.Fatalf("local validation: %+v", s)
		case s.Kind == "order" && (s.Remote == "" || s.Intent != "" || s.Text == ""):
			t.Fatalf("remote order: %+v", s)
		}
	}
	var untraced bool
	for _, x := range list {
		untraced = untraced || (x.Untraced && x.ID == "run:"+tracefixture.OrderLegacy)
	}
	if !untraced {
		t.Fatal("Oven's untraced run is listed on the mini")
	}

	// Oven's dashboard sees the mini's validation run the same way.
	tr, spans, _ = read(oven, shared.all())
	if k := kinds(spans); tr.Runs != 4 || k["validation"] != 1 || tr.State != "done" {
		t.Fatalf("oven trace: %+v %v", tr, k)
	}

	// A worker session opened on the mini leads back to Oven's trace.
	h := New(&MemoryReader{Sessions: sessions, Blocks: blocks, Spans: shared.all()}).WithDispatchRoot(mini).Handler()
	var session struct {
		TraceContext *TraceContext `json:"trace_context"`
	}
	getAPI(t, h, "/api/session/11111111-0000-4000-8000-000000000002", &session)
	if c := session.TraceContext; c == nil || c.Trace != info.Trace || c.Launch != "rework" || c.Order != tracefixture.OrderNav || c.RunSpan != "b2b2b2b2b2b2b2b2" {
		t.Fatalf("remote session context: %+v", c)
	}
	// Run keys match across hosts, so a host never shows its own run twice.
	for _, r := range dispatch.ReadRunRecords(oven) {
		found := false
		for _, row := range shared.all() {
			found = found || row.RunKey == dispatch.RunKey(r)
		}
		if !found {
			t.Fatalf("run %s not published under its key", r.Name)
		}
	}
}
