package index

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/tracefixture"
)

// Indexing one trace that spans two orders, a rework and a validation run
// stamps every worker session row with that one trace ID and the span of the
// attempt that launched it. The untraced run's session carries none.
func TestIndexJoinsWorkerSessionsToTrace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	info, err := tracefixture.Write(home, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	rows := map[string]map[string]any{}
	srv := httptest.NewServer(withSessionPersistence(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{}`)
			return
		}
		var body struct {
			Rows []map[string]any `json:"upsert_rows"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/v2/namespaces/namespace-sessions" {
			mu.Lock()
			for _, row := range body.Rows {
				rows[row["session_id"].(string)] = row
			}
			mu.Unlock()
		}
		fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
	})))
	defer srv.Close()
	rep, err := run(&trace.ClaudeSource{Root: info.Transcripts}, layer.New(srv.URL, "key", "namespace", ""), &State{Units: map[string]string{}}, Options{RepoURL: func(string) string { return "" }})
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	want := map[string][2]string{
		"11111111-0000-4000-8000-000000000001": {tracefixture.OrderNav, "b1b1b1b1b1b1b1b1"},
		"11111111-0000-4000-8000-000000000002": {tracefixture.OrderNav, "b2b2b2b2b2b2b2b2"},
		"11111111-0000-4000-8000-000000000003": {tracefixture.OrderNav, "c1c1c1c1c1c1c1c1"},
		"11111111-0000-4000-8000-000000000004": {tracefixture.OrderSearch, "d1d1d1d1d1d1d1d1"},
	}
	traces := map[any]bool{}
	for id, w := range want {
		row := rows[id]
		if row == nil || row["trace_id"] != info.Trace || row["order"] != w[0] || row["parent_span_id"] != w[1] {
			t.Errorf("session %s: %v", id, row)
		}
		traces[row["trace_id"]] = true
	}
	if len(traces) != 1 {
		t.Fatalf("two orders read as %d traces", len(traces))
	}
	legacy := rows["11111111-0000-4000-8000-000000000006"]
	if legacy == nil || legacy["trace_id"] != nil || legacy["parent_span_id"] != nil || legacy["order"] != tracefixture.OrderLegacy {
		t.Fatalf("untraced session: %v", legacy)
	}

	// The in-memory read side derives the same join.
	sessions, blocks, err := ReadSide(&trace.ClaudeSource{Root: info.Transcripts}, "fixture")
	if err != nil || len(sessions) != 6 || len(blocks) == 0 {
		t.Fatalf("read side: %d sessions %d blocks %v", len(sessions), len(blocks), err)
	}
	for _, s := range sessions {
		if w, ok := want[s.SessionID]; ok && (s.TraceID != info.Trace || s.ParentSpanID != w[1]) {
			t.Fatalf("read side %s: %+v", s.SessionID, s)
		}
	}
}
