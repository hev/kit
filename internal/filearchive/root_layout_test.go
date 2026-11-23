package filearchive

import (
	"os"
	"path/filepath"
	"testing"
)

// The archive root rule from both sides: a fresh HOME gets ~/.hev/traces, a
// HOME with only the legacy ~/.hev/archive keeps it, and [archive] dir and
// HEV_ARCHIVE_DIR win over both (the variable over the config).
func TestRootUnifiedLayoutAndLegacyFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(DirEnv, "")
	if got, want := Root(""), filepath.Join(home, ".hev", "traces"); got != want {
		t.Fatalf("fresh: %s, want %s", got, want)
	}
	legacy := filepath.Join(home, ".hev", "archive")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := Root(""); got != legacy {
		t.Fatalf("legacy: %s, want %s", got, legacy)
	}
	configured := filepath.Join(home, "configured")
	if got := Root(configured); got != configured {
		t.Fatalf("config: %s", got)
	}
	env := filepath.Join(home, "env")
	t.Setenv(DirEnv, env)
	if got := Root(configured); got != env {
		t.Fatalf("env over config: %s", got)
	}
	if got := Root(""); got != env {
		t.Fatalf("env over legacy: %s", got)
	}
}
