package index

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// After indexing, each worker session row carries the context digest its
// attempt was given, and sessions can be filtered by it.
func TestIndexWritesContextDigestToSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	dir := filepath.Join(dispatch.Root(), "ctx-run")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "order.md"), []byte("---\nname: ctx-run\nrepo: project\ncontext: recipe.json\n---\n## Acceptance\n"), 0600)
	// Attempt 1 (archived) ran session s1 with the first bundle; attempt 2,
	// current, ran s2 with the second.
	dispatch.WriteJSON(filepath.Join(dir, "result.json.1"), dispatch.Result{Session: "s1", ContextDigest: first})
	dispatch.WriteJSON(filepath.Join(dir, "result.json"), dispatch.Result{Session: "s2", ContextDigest: second})
	dispatch.WriteJSON(filepath.Join(dir, "meta.json"), dispatch.Meta{Order: dispatch.Order{Name: "ctx-run", Context: "recipe.json"}, ContextDigest: second})
	other := t.TempDir()

	source := t.TempDir()
	var lines []string
	for i, s := range []struct{ id, cwd string }{{"s1", filepath.Join(dir, "wt")}, {"s2", filepath.Join(dir, "wt")}, {"s3", other}} {
		lines = append(lines, fmt.Sprintf(`{"type":"user","uuid":"u%d","sessionId":%q,"cwd":%q,"timestamp":"2026-11-12T10:0%d:00Z","message":{"role":"user","content":"hello"}}`, i, s.id, s.cwd, i))
	}
	for i, line := range lines {
		os.WriteFile(filepath.Join(source, fmt.Sprintf("session%d.jsonl", i)), []byte(line+"\n"), 0600)
	}
	srv := httptest.NewServer(withSessionPersistence(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{}`)
			return
		}
		fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
	})))
	defer srv.Close()
	cl := layer.New(srv.URL, "key", "namespace", "")
	rep, err := run(&trace.ClaudeSource{Root: source}, cl, &State{Units: map[string]string{}}, Options{})
	if err != nil || len(rep.Errors) > 0 || rep.Sessions != 3 {
		t.Fatalf("report=%+v err=%v", rep, err)
	}
	got := map[string]string{}
	all, err := cl.ListSessionRows(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range all {
		got[row.SessionID] = row.ContextDigest
	}
	if got["s1"] != first || got["s2"] != second || got["s3"] != "" {
		t.Fatalf("digests %+v", got)
	}
	for digest, want := range map[string]string{first: "s1", second: "s2"} {
		rows, err := cl.ListSessionRows(100, []any{"context_digest", "Eq", digest})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, r := range rows {
			ids = append(ids, r.SessionID)
		}
		sort.Strings(ids)
		if strings.Join(ids, ",") != want {
			t.Fatalf("filter %s: %v", digest[:4], ids)
		}
	}
}
