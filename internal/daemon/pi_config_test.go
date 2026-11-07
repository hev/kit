package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPiRootConfigAndEnvironmentPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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

func TestOpenCodeRootConfigAndEnvironmentPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_OPENCODE_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	dir := filepath.Join(home, ".hev")
	os.MkdirAll(dir, 0700)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenCodeDataDir != filepath.Join(home, ".local", "share", "opencode") {
		t.Fatalf("default root %q", cfg.OpenCodeDataDir)
	}
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[capture]\nopencode_data_dir = '~/isolated-opencode'\n"), 0600)
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenCodeDataDir != filepath.Join(home, "isolated-opencode") {
		t.Fatalf("config root %q", cfg.OpenCodeDataDir)
	}
	t.Setenv("HEV_OPENCODE_DATA_DIR", "/synthetic/env-opencode")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenCodeDataDir != "/synthetic/env-opencode" {
		t.Fatalf("env root %q", cfg.OpenCodeDataDir)
	}
}
