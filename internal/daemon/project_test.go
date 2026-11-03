package daemon

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
)

func TestProjectBlockedStatusSurvivesUnchangedCycleAndRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	root := filepath.Join(home, ".claude", "projects", "project")
	project := filepath.Join(home, "project")
	for _, dir := range []string{root, project} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, ".hev.toml"), []byte("[capture]\nnamespace='refused'"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf(`{"type":"user","uuid":"u","sessionId":"s","cwd":%q,"message":{"role":"user","content":"synthetic blocked session"}}`, project) + "\n"
	if err := os.WriteFile(filepath.Join(root, "session.jsonl"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && !strings.HasSuffix(r.URL.Path, "/query") {
			writes++
		}
		io.WriteString(w, `{"rows":[]}`)
	}))
	defer srv.Close()
	cl := layer.New(srv.URL, "key", "default", "")
	state := index.LoadState()
	for i := 0; i < 2; i++ {
		status := runIndexCycle(cl, state, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if status.LastError != "" || status.BlockedByProjectConfig != 1 || writes != 0 {
			t.Fatalf("cycle %d: %+v writes=%d", i, status, writes)
		}
		if err := writeStatus(status); err != nil {
			t.Fatal(err)
		}
		saved, err := ReadStatus()
		if err != nil || saved.BlockedByProjectConfig != 1 {
			t.Fatalf("status: %+v %v", saved, err)
		}
		state = index.LoadState()
	}
}
