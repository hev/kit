package who

import (
	"github.com/hev/kit/internal/board/settings"
	"os"
	"path/filepath"
	"testing"
)

func clean(t *testing.T) {
	t.Helper()
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current = settings.Config{}
	for _, key := range []string{"FACTORY_SESSION", "FACTORY_BOARD_AS", "TMUX", "FACTORY_ROLE", "FACTORY_INSTANCE", "CLAUDE_SESSION_ID", "CLAUDECODE", "CODEX_THREAD_ID", "CODEX_CI"} {
		t.Setenv(key, "")
	}
	t.Setenv("HOME", t.TempDir())
}
func TestSessionProvenanceAndOverrides(t *testing.T) {
	clean(t)
	settings.Current.Identity = "fallback"
	settings.Current.Roles = []settings.Role{{Pattern: "^coordinator(?:-(.+))?$", Role: "coordinator", Instance: "$1"}}
	t.Setenv("FACTORY_SESSION", "build-422")
	for _, harness := range []string{"", "harness-id"} {
		t.Setenv("CLAUDE_SESSION_ID", harness)
		id := Detect("")
		if id.Author != "factory-build-422" || id.Role != "worker" || id.Session != "build-422" {
			t.Fatalf("session: %+v", id)
		}
		id = Detect("coordinator-project")
		if id.Role != "coordinator" || id.Instance != "project" || id.Session != "build-422" {
			t.Fatalf("override: %+v", id)
		}
	}
	t.Setenv("FACTORY_BOARD_AS", "environment")
	if id := Detect(""); id.Author != "environment" {
		t.Fatal(id)
	}
}
func TestHeadlessIdentity(t *testing.T) {
	for _, marker := range []string{"config", ".hev", ".factory"} {
		t.Run(marker, func(t *testing.T) {
			clean(t)
			if marker == "config" {
				settings.Current.Headless = true
			} else {
				dir := filepath.Join(os.Getenv("HOME"), marker)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "no-desk"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if id := Detect(""); id.Role != "agent" || id.Author != "agent" {
				t.Fatal(id)
			}
		})
	}
}
func TestConfiguredRolesGolden(t *testing.T) {
	clean(t)
	settings.Current.Roles = []settings.Role{{Pattern: "^coordinator(?:-(.+))?$", Role: "coordinator", Instance: "$1"}, {Pattern: "^worker-([^-]+).*$", Role: "worker", Instance: "$1"}}
	for _, c := range []struct{ name, role, instance string }{{"coordinator", "coordinator", ""}, {"coordinator-project", "coordinator", "project"}, {"worker-project-index", "worker", "project"}} {
		id := Detect(c.name)
		if id.Author != c.name || id.Role != c.role || id.Instance != c.instance {
			t.Fatalf("legacy row mismatch: %+v", id)
		}
	}
}
