package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/storagemode"
)

// isolate gives each case its own HOME, config and archive root, with no
// key anywhere in the environment.
func isolate(t *testing.T, config string) (*Config, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_SECRETS_CACHE", "off")
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
	}
	t.Setenv("HEV_ARCHIVE_DIR", "")
	path := filepath.Join(home, ".hev", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if config != "" {
		if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HEV_CONFIG", path)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg, filepath.Join(home, ".hev", "archive")
}

func TestOpenTargetNoKeyIsFiles(t *testing.T) {
	cfg, root := isolate(t, "[layer]\nendpoint = \"https://example.invalid\"\napi_key = \"\"\n")
	target, err := OpenTarget(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Mode != storagemode.Filesystem || target.Files == nil || target.Layer != nil || target.Retired {
		t.Fatalf("target = %+v", target)
	}
	if want := filepath.Join(root, "hev-traces"); target.Files.Dir() != want {
		t.Fatalf("archive dir = %s, want %s", target.Files.Dir(), want)
	}
}

func TestOpenTargetArchiveDirFromConfigAndEnv(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "configured")
	cfg, _ := isolate(t, "[archive]\ndir = \""+filepath.ToSlash(dir)+"\"\n")
	target, err := OpenTarget(cfg, "ns")
	if err != nil {
		t.Fatal(err)
	}
	if target.Files.Dir() != filepath.Join(dir, "ns") {
		t.Fatalf("dir = %s", target.Files.Dir())
	}
	env := filepath.Join(t.TempDir(), "env")
	t.Setenv("HEV_ARCHIVE_DIR", env)
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	target, err = OpenTarget(cfg, "")
	if err != nil || target.Files.Dir() != filepath.Join(env, "hev-traces") {
		t.Fatalf("env dir = %+v, %v", target, err)
	}
}

func TestOpenTargetPlaceholderKeyIsNoKeyAndRetiredStoreIsReported(t *testing.T) {
	cfg, _ := isolate(t, "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"local\"\nstore = \""+RetiredStore+"\"\nnamespace = \"hev-traces\"\n[local]\nproject = \"hev-kit\"\n")
	target, err := OpenTarget(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Mode != storagemode.Filesystem || !target.Retired {
		t.Fatalf("target = %+v", target)
	}
	msg := RetiredMessage(cfg)
	for _, want := range []string{"retired", "hev backfill", "docker compose -p hev-kit down", "docker volume rm", "files under"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("retirement message lacks %q:\n%s", want, msg)
		}
	}
	// The placeholder in the environment is not a key either.
	t.Setenv("HEVLAYER_API_KEY", "local")
	target, err = OpenTarget(cfg, "")
	if err != nil || target.Mode != storagemode.Filesystem {
		t.Fatalf("env placeholder: %+v, %v", target, err)
	}
}

func TestOpenTargetWithKeyIsLayerAndWritesNoFiles(t *testing.T) {
	cfg, root := isolate(t, "[layer]\nendpoint = \"https://gateway.example\"\napi_key = \"saved-key\"\nnamespace = \"team\"\n")
	target, err := OpenTarget(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Mode != storagemode.Layer || target.Layer == nil || target.Files != nil {
		t.Fatalf("target = %+v", target)
	}
	if target.Layer.APIKey != "saved-key" || target.Layer.Endpoint != "https://gateway.example" || target.Layer.Namespace != "team" {
		t.Fatalf("client = %s %s", target.Layer.Endpoint, target.Layer.Namespace)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("archive directory created in Layer mode: %v", err)
	}
	t.Setenv("HEVLAYER_API_KEY", "env-key")
	cfg, _ = LoadConfig()
	target, err = OpenTarget(cfg, "")
	if err != nil || target.Layer.APIKey != "env-key" {
		t.Fatalf("env key: %v", err)
	}
}

func TestOpenTargetUnreadableKeyIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake op is a shell script")
	}
	cfg, root := isolate(t, "")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "op"), []byte("#!/bin/sh\necho denied >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://vault/item/field")
	if _, err := OpenTarget(cfg, ""); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("err = %v", err)
	}
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "")
	t.Setenv("HEVLAYER_API_KEY", "  ")
	if _, err := OpenTarget(cfg, ""); err == nil {
		t.Fatal("blank key accepted")
	}
	t.Setenv("HEVLAYER_API_KEY", "")
	if err := os.WriteFile(cfg.ConfigPath, []byte("[layer\napi_key = "), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenTarget(cfg, ""); err == nil {
		t.Fatal("unparseable config treated as no key")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("archive created after a key error: %v", err)
	}
}
