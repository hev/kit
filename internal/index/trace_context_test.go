package index

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// A worker session from a run that was both traced and given a context
// recipe gets the trace, the attempt span and the context digest on its one
// session row, in hev layer and in files, and each filter finds it.
func TestIndexSessionCarriesTraceAndContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	traceID := strings.Repeat("1", 32)
	first, second := strings.Repeat("a", 16), strings.Repeat("b", 16)
	digest1, digest2 := strings.Repeat("c", 64), strings.Repeat("d", 64)
	dir := filepath.Join(dispatch.Root(), "both-run")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte("---\nname: both-run\nrepo: project\ncontext: recipe.json\n---\n## Acceptance\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Attempt 1 (archived) ran s1; attempt 2 (current), after a rework, ran
	// s2 with a new span and a new bundle.
	for path, v := range map[string]any{
		"result.json.1": dispatch.Result{Session: "s1", TraceID: traceID, SpanID: first, ContextDigest: digest1},
		"result.json":   dispatch.Result{Session: "s2", TraceID: traceID, SpanID: second, ContextDigest: digest2},
		"meta.json":     dispatch.Meta{Order: dispatch.Order{Name: "both-run", Context: "recipe.json"}, TraceID: traceID, SpanID: second, ContextDigest: digest2},
	} {
		if err := dispatch.WriteJSON(filepath.Join(dir, path), v); err != nil {
			t.Fatal(err)
		}
	}
	source := t.TempDir()
	for i, id := range []string{"s1", "s2"} {
		line := fmt.Sprintf(`{"type":"user","uuid":"u%d","sessionId":%q,"cwd":%q,"timestamp":"2026-11-12T10:0%d:00Z","message":{"role":"user","content":"hello"}}`, i, id, filepath.Join(dir, "wt"), i)
		if err := os.WriteFile(filepath.Join(source, id+".jsonl"), []byte(line+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string][3]string{"s1": {traceID, first, digest1}, "s2": {traceID, second, digest2}}

	srv := httptest.NewServer(withSessionPersistence(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{}`)
			return
		}
		fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
	})))
	defer srv.Close()
	files, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	type store interface {
		Archive
		ListSessionRows(int, any) ([]trace.SessionRow, error)
	}
	for name, cl := range map[string]store{"layer": layer.New(srv.URL, "key", "namespace", ""), "files": files} {
		rep, err := run(&trace.ClaudeSource{Root: source}, cl, &State{Units: map[string]string{}}, Options{RepoURL: func(string) string { return "" }})
		if err != nil || len(rep.Errors) > 0 || rep.Sessions != 2 {
			t.Fatalf("%s: report=%+v err=%v", name, rep, err)
		}
		rows, err := cl.ListSessionRows(-1, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string][3]string{}
		for _, r := range rows {
			got[r.SessionID] = [3]string{r.TraceID, r.ParentSpanID, r.ContextDigest}
		}
		for id, w := range want {
			if got[id] != w {
				t.Errorf("%s: session %s carries %v, want %v", name, id, got[id], w)
			}
		}
		for _, f := range [][3]any{{"trace_id", "Eq", traceID}, {"context_digest", "Eq", digest2}} {
			rows, err := cl.ListSessionRows(-1, []any{f[0], f[1], f[2]})
			if err != nil {
				t.Fatal(err)
			}
			n := map[string]int{"trace_id": 2, "context_digest": 1}[f[0].(string)]
			if len(rows) != n {
				t.Errorf("%s: filter %v found %d rows, want %d", name, f[0], len(rows), n)
			}
		}
	}
}
