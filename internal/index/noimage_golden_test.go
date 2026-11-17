package index

import (
	"encoding/json"
	"flag"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

var updateNoImageGolden = flag.Bool("update-noimage-golden", false, "rewrite testdata/noimage-rows.golden.json")

// TestNoImageRowsMatchBase pins what a session without images writes, in
// both storage modes, to the bytes the branch before image capture wrote.
// The golden file was generated on that branch with -update-noimage-golden;
// this test uses only APIs that existed there.
func TestNoImageRowsMatchBase(t *testing.T) {
	claudeRoot, codexRoot := t.TempDir(), t.TempDir()
	write := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(claudeRoot, "-w", "0f0f0f0f-1111-4222-8333-444455556666.jsonl"),
		`{"type":"user","uuid":"u1","sessionId":"0f0f0f0f-1111-4222-8333-444455556666","timestamp":"2026-11-10T10:00:00Z","cwd":"/w","message":{"role":"user","content":[{"type":"text","text":"run the tests"}]}}`+"\n"+
			`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"0f0f0f0f-1111-4222-8333-444455556666","timestamp":"2026-11-10T10:00:02Z","cwd":"/w","requestId":"r1","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}],"usage":{"input_tokens":3,"output_tokens":4}}}`+"\n"+
			`{"type":"user","uuid":"u2","parentUuid":"a1","sessionId":"0f0f0f0f-1111-4222-8333-444455556666","timestamp":"2026-11-10T10:00:04Z","cwd":"/w","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"ok  pkg 0.1s"},{"type":"text","text":"ok  other 0.2s"}]}]}}`+"\n"+
			`{"type":"assistant","uuid":"a2","parentUuid":"u2","sessionId":"0f0f0f0f-1111-4222-8333-444455556666","timestamp":"2026-11-10T10:00:06Z","cwd":"/w","requestId":"r2","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"all green"}],"usage":{"input_tokens":5,"output_tokens":6}}}`+"\n")
	write(filepath.Join(codexRoot, "2026", "11", "10", "rollout-c.jsonl"),
		`{"timestamp":"2026-11-10T10:00:00Z","type":"session_meta","payload":{"id":"0c0c0c0c-1111-4222-8333-444455556666","cwd":"/w"}}`+"\n"+
			`{"timestamp":"2026-11-10T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","id":"m1","content":[{"type":"input_text","text":"list the files"}]}}`+"\n"+
			`{"timestamp":"2026-11-10T10:00:02Z","type":"response_item","payload":{"type":"function_call","id":"f1","call_id":"c1","name":"shell","arguments":"{\"command\":[\"ls\"]}"}}`+"\n"+
			`{"timestamp":"2026-11-10T10:00:03Z","type":"response_item","payload":{"type":"function_call_output","id":"o1","call_id":"c1","output":[{"type":"input_text","text":"a.go"},{"type":"input_text","text":"b.go"}]}}`+"\n"+
			`{"timestamp":"2026-11-10T10:00:04Z","type":"response_item","payload":{"type":"message","role":"assistant","id":"m2","content":[{"type":"output_text","text":"two files"}]}}`+"\n")
	sources := []trace.Source{&trace.ClaudeSource{Root: claudeRoot}, &trace.CodexSource{Root: codexRoot}}
	host := layer.Hostname()
	norm := func(s string) string {
		s = strings.ReplaceAll(s, claudeRoot, "CLAUDE_ROOT")
		s = strings.ReplaceAll(s, codexRoot, "CODEX_ROOT")
		if host != "" {
			s = strings.ReplaceAll(s, `"`+host+`"`, `"HOST"`)
		}
		return s
	}
	got := map[string]string{}

	// Files.
	store, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range sources {
		if rep, err := Run(src, store, &State{Units: map[string]string{}}, Options{RepoURL: func(string) string { return "" }, Tiers: trace.AllTiers[:]}); err != nil || len(rep.Errors) != 0 {
			t.Fatalf("file run = %+v, %v", rep, err)
		}
	}
	filepath.WalkDir(store.Dir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(store.Dir(), p)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "sessions/") || strings.HasPrefix(rel, "blocks/") || strings.HasPrefix(rel, "chunks/") {
			b, _ := os.ReadFile(p)
			got["files/"+rel] = norm(string(b))
		}
		return nil
	})

	// Layer.
	var mu sync.Mutex
	srv := httptest.NewServer(withSessionPersistence(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.HasSuffix(r.URL.Path, "/query") || r.Method != "POST" {
			io.WriteString(w, `{"rows":[]}`)
			return
		}
		mu.Lock()
		got["layer"+r.URL.Path] += norm(string(b)) + "\n"
		mu.Unlock()
		io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()
	cl := layer.New(srv.URL, "key", "ns", "")
	for _, src := range sources {
		if rep, err := Run(src, cl, &State{Units: map[string]string{}}, Options{RepoURL: func(string) string { return "" }, Tiers: trace.AllTiers[:]}); err != nil || len(rep.Errors) != 0 {
			t.Fatalf("layer run = %+v, %v", rep, err)
		}
	}

	golden := filepath.Join("testdata", "noimage-rows.golden.json")
	if *updateNoImageGolden {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for k := range want {
		keys[k] = true
	}
	for k := range got {
		keys[k] = true
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if got[k] != want[k] {
			t.Errorf("%s differs from the base branch:\n got: %s\nwant: %s", k, got[k], want[k])
		}
	}
}
