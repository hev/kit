package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPiRootConfigAndEnvironmentPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_PI_SESSIONS_DIR", "")
	dir := filepath.Join(home, ".hev")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[capture]\npi_sessions_dir = '~/isolated-pi'\n"), 0600)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PiSessionsDir != filepath.Join(home, "isolated-pi") {
		t.Fatalf("config root %q", cfg.PiSessionsDir)
	}
	t.Setenv("HEV_PI_SESSIONS_DIR", "/synthetic/env-pi")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PiSessionsDir != "/synthetic/env-pi" {
		t.Fatalf("env root %q", cfg.PiSessionsDir)
	}
}
