package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boardsettings "github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/internal/license"
	"github.com/hev/kit/internal/serve"
	"github.com/hev/kit/internal/testenv"
)

func TestServeWorkspaceUsesConfiguredProSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("TMUX_TMPDIR", testenv.SocketDir(t))
	t.Setenv("HEV_LICENSE", "")
	t.Setenv("HEV_LICENSE_REF", "")
	t.Setenv("FACTORY_BOARD_URL", "")
	t.Setenv(evidence.DirEnv, "")
	t.Setenv("HEV_WORKSPACE", "")
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	orders := filepath.Join(home, "configured-orders")
	t.Setenv("HEV_ORDERS_DIR", orders)
	private := filepath.Join(home, "operator-private")
	const key = "configured-auth-value-fake"
	previous := boardsettings.Current
	t.Cleanup(func() { boardsettings.Current = previous })
	board := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" || r.URL.Query().Get("q") != "needle" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"title": "needle board", "detail": private + " " + key}})
	}))
	defer board.Close()
	evidenceDir := filepath.Join(home, "configured-evidence")
	config := fmt.Sprintf("[board]\nurl = %q\n[evidence]\ndir = %q\n", board.URL, evidenceDir)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	ev, err := evidence.Open(evidenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ev.Create(evidence.Meta{Title: "needle evidence", Summary: private}); err != nil {
		t.Fatal(err)
	}
	writeJSON := func(path string, value any) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(filepath.Join(orders, "needle-order", "meta.json"), dispatch.Meta{Order: dispatch.Order{Name: "needle-order", Repo: private, Harness: "codex"}})
	writeJSON(filepath.Join(orders, "foreign-order", "meta.json"), dispatch.Meta{WorkspaceName: "another-workspace", Order: dispatch.Order{Name: "foreign-order", Harness: "codex"}})
	build := func() http.Handler {
		return serveLocalWorkspace(t.Context(), io.Discard, serve.New(&serve.MemoryReader{}), key).Handler()
	}
	check := func(handler http.Handler, path string, code int, want string) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		body := rec.Body.String()
		if rec.Code != code || !strings.Contains(body, want) {
			t.Fatalf("GET %s: returned %d, want %d containing %q", path, rec.Code, code, want)
		}
		if strings.Contains(body, key) || strings.Contains(body, private) || strings.Contains(body, "foreign-order") {
			t.Fatalf("GET %s exposed private source metadata", path)
		}
	}
	check(build(), "/api/evidence?q=needle", 403, "requires Kit Pro")
	now := time.Now()
	expiry := now.Add(time.Hour)
	writeJSON(filepath.Join(home, ".hev", "license-status.json"), license.Status{
		Valid: true, Tier: "pro", Expires: &expiry, CheckedAt: now.Add(-time.Minute), Until: expiry,
	})
	pro := build()
	check(pro, "/api/board/search?q=needle", 200, "needle board")
	check(pro, "/api/evidence?q=needle", 200, "needle evidence")
	check(pro, "/api/orders?q=needle", 200, "needle-order")
	check(pro, "/api/peers", 200, "d-needle-order")
	check(pro, "/api/settings", 200, "Cluster")
}
