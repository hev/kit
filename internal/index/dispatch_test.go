package index

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDispatchAttributionOnChunksAndSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(dispatch.Root(), "example")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "order.md"), []byte("---\nname: example\nrepo: project\nissue: TASK-42\n---\n## Acceptance\n- done\n"), 0600); e != nil {
		t.Fatal(e)
	}
	turn := trace.Turn{SessionID: "session", Workdir: filepath.Join(dir, "wt"), Blocks: []trace.Block{{Type: "text", Text: "hello"}}}
	attribute := unitAttribution(func(trace.Turn) (string, string, string, string, string) {
		return "instance", "plan", "rfc", "other-issue", "pr"
	})
	a := attribute(turn)
	sessions := trace.Sessions([]trace.Turn{turn}, nil, "")
	if a.order != "example" || a.issue != "TASK-42" || a.plan != "plan" || len(sessions) != 1 || sessions[0].Order != "" {
		t.Fatalf("attribute=%+v sessions=%+v", a, sessions)
	}
	_ = os.Remove(filepath.Join(dir, "order.md"))
	if got := attribute(turn); got.issue != "TASK-42" {
		t.Fatalf("cache lost: %+v", got)
	}
}

func TestIndexWritesOrderToChunksAndSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(dispatch.Root(), "example")
	os.MkdirAll(filepath.Join(dir, "wt"), 0700)
	os.WriteFile(filepath.Join(dir, "order.md"), []byte("---\nname: example\nrepo: project\nissue: TASK-42\n---\n## Acceptance\n"), 0600)
	source := t.TempDir()
	line := fmt.Sprintf(`{"type":"user","uuid":"u1","sessionId":"s1","cwd":%q,"timestamp":"2026-09-05T10:00:00Z","message":{"role":"user","content":"hello"}}`, filepath.Join(dir, "wt"))
	os.WriteFile(filepath.Join(source, "session.jsonl"), []byte(line+"\n"), 0600)
	var chunks, sessions int
	srv := httptest.NewServer(withSessionPersistence(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{}`)
			return
		}
		var body struct {
			Rows []map[string]any `json:"upsert_rows"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path == "/v2/namespaces/namespace" || r.URL.Path == "/v2/namespaces/namespace-sessions" {
			for _, row := range body.Rows {
				if row["order"] != "example" || row["issue"] != "TASK-42" {
					t.Errorf("attribution missing: %+v", row)
				}
				if r.URL.Path == "/v2/namespaces/namespace" {
					chunks++
				} else {
					sessions++
				}
			}
		}
		fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
	})))
	defer srv.Close()
	rep, err := run(&trace.ClaudeSource{Root: source}, layer.New(srv.URL, "key", "namespace", ""), &State{Units: map[string]string{}}, Options{Attribution: func(trace.Turn) (string, string, string, string, string) { return "", "", "", "other", "" }})
	if err != nil || len(rep.Errors) > 0 || chunks != 1 || sessions != 1 {
		t.Fatalf("chunks=%d sessions=%d report=%+v err=%v", chunks, sessions, rep, err)
	}
}
