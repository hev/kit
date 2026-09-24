package index

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestSummarizeWritesOnlySessionRowsAndPreservesHarnessTitle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	lines := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-09-05T10:00:00Z","message":{"role":"user","content":"fix it"}}` + "\n" +
		`{"type":"ai-title","sessionId":"s1","aiTitle":"Harness title"}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	var request map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/namespaces/namespace-sessions" {
			t.Errorf("write path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()
	called := false
	rep, err := Summarize(&trace.ClaudeSource{Root: root}, layer.New(srv.URL, "key", "namespace", ""), 200, nil, nil, func(trace.SessionRow) (string, error) {
		called = true
		return "generated", nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("generator called despite harness title")
	}
	if rep.SessionRowsUpserted != 1 || rep.Sessions != 1 {
		t.Fatalf("report = %+v", rep)
	}
	var rows []trace.SessionRow
	if err := json.Unmarshal(request["patch_rows"], &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Summary != "Harness title" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestRunSkipsMalformedCodexUnit(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "bad.jsonl")
	good := filepath.Join(root, "good.jsonl")
	if err := os.WriteFile(bad, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good, []byte(
		`{"timestamp":"2026-09-05T10:00:00Z","type":"session_meta","payload":{"id":"s1"}}`+"\n"+
			`{"timestamp":"2026-09-05T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(&trace.CodexSource{Root: root}, nil, &State{Units: map[string]string{}}, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Run failed instead of skipping malformed unit: %v", err)
	}
	if rep.UnitsSeen != 2 || rep.UnitsIndexed != 1 || len(rep.Errors) != 1 {
		t.Fatalf("report = %+v, want one indexed unit and one error", rep)
	}
}

func TestRunSkipsUnchangedCodexUnitWithoutEmbedding(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(
		`{"timestamp":"2026-09-05T10:00:00Z","type":"session_meta","payload":{"id":"s1"}}`+"\n"+
			`{"timestamp":"2026-09-05T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	writes := 0
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writes++
		paths = append(paths, r.URL.Path)
		io.WriteString(w, `{"status":"OK","rows_upserted":1,"performance":{"embedding_tokens":7}}`)
	}))
	defer srv.Close()
	cl := layer.New(srv.URL, "key", "namespace", "")
	state := &State{Units: map[string]string{}}
	src := &trace.CodexSource{Root: root}

	first, err := Run(src, cl, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Run(src, cl, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if first.UnitsIndexed != 1 || first.EmbeddingTokens != 7 || writes != 3 {
		t.Fatalf("first report = %+v, writes = %d", first, writes)
	}
	wantPaths := []string{
		"/v2/namespaces/namespace",
		"/v2/namespaces/namespace-blocks",
		"/v2/namespaces/namespace-sessions",
	}
	for i, want := range wantPaths {
		if paths[i] != want {
			t.Fatalf("write %d path = %q, want %q", i, paths[i], want)
		}
	}
	if second.UnitsSkipped != 1 || second.UnitsIndexed != 0 || second.EmbeddingTokens != 0 || second.RowsUpserted != 0 || writes != 3 {
		t.Fatalf("second report = %+v, want unchanged unit and zero write cost", second)
	}
}

func TestRunCollapsesDuplicateChunkIDsInOneUpsert(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	// One user turn carrying the same text block twice hashes to one chunk id.
	lines := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-09-05T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"[Image #1]"},{"type":"text","text":"[Image #1]"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	var ids []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/namespaces/namespace" {
			var body struct {
				Rows []struct {
					ID string `json:"id"`
				} `json:"upsert_rows"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			for _, row := range body.Rows {
				ids = append(ids, row.ID)
			}
		}
		io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()

	rep, err := Run(&trace.ClaudeSource{Root: root}, layer.New(srv.URL, "key", "namespace", ""), &State{Units: map[string]string{}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) != 0 || rep.UnitsIndexed != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if len(ids) != 1 || rep.Chunks != 1 {
		t.Fatalf("upserted ids = %v, chunks = %d; want the duplicate collapsed to one row", ids, rep.Chunks)
	}
}
