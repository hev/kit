package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sandboxConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", path)
	for _, env := range []string{"LAYER_ENDPOINT", "LAYER_API_KEY", "LAYER_NAMESPACE", "LAYER_STORE", "HEV_LOCAL_IMAGE", "HEV_LOCAL_PROJECT", "HEV_LOCAL_PORT", "HEV_LOCAL_SERVE_PORT", "HEV_LOCAL_KIT_IMAGE"} {
		t.Setenv(env, "")
	}
	return path
}

const testKey = "tpuf_test_key"

func TestWriteLocalConfigFromNothingThenNoOp(t *testing.T) {
	path := sandboxConfig(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Local.Managed {
		t.Fatal("a machine with no config is not managed")
	}
	if cfg.Local.Image != DefaultLocalImage || cfg.Local.Port != 8080 || cfg.Local.Project != DefaultLocalProject {
		t.Fatalf("defaults = %+v", cfg.Local)
	}
	if w, err := WriteLocalConfig(cfg.Local, testKey); err != nil || !w.Changed || w.Migrated {
		t.Fatalf("first write: %+v err=%v", w, err)
	}
	info, _ := os.Stat(path)

	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Local.Managed || cfg.LayerEndpoint != "http://127.0.0.1:8080" || cfg.LayerAPIKey != testKey ||
		cfg.LayerNamespace != "hev-traces" || cfg.LayerStore != "turbopuffer" || cfg.Local.KitImage != "hevlayer/kit:latest" {
		t.Fatalf("config = %+v", cfg)
	}
	if err := validateLayerTarget(cfg); err != nil {
		t.Fatalf("daemon would refuse the config up wrote: %v", err)
	}

	if LocalAPIKey() != testKey {
		t.Fatalf("stored key not read back: %q", LocalAPIKey())
	}
	if w, err := WriteLocalConfig(cfg.Local, testKey); err != nil || w.Changed {
		t.Fatalf("second write: %+v err=%v", w, err)
	}
	again, _ := os.Stat(path)
	if !again.ModTime().Equal(info.ModTime()) {
		t.Fatal("an unchanged config was rewritten")
	}
}

// The pin, the port and the project are each one value in the file.
func TestLocalBlockIsReadBack(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("[local]\nimage = \"hevlayer/layer-gateway:v0.6.1\"\nport = 9191\nproject = \"mine\"\nkit_image = \"hevlayer/kit:0.1.0\"\n"), 0o600)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := LocalConfig{Managed: true, Image: "hevlayer/layer-gateway:v0.6.1", Port: 9191, Project: "mine", KitImage: "hevlayer/kit:0.1.0", ServePort: DefaultLocalServePort}
	if cfg.Local != want {
		t.Fatalf("local = %+v, want %+v", cfg.Local, want)
	}
	t.Setenv("HEV_LOCAL_PORT", "9292")
	if cfg, _ = LoadConfig(); cfg.Local.Port != 9292 {
		t.Fatalf("env override ignored: %d", cfg.Local.Port)
	}
	t.Setenv("HEV_LOCAL_PORT", "http")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("a non-numeric port was accepted")
	}
}

func TestWriteLocalConfigRefusesAHostedConfig(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	hosted := "[layer]\nendpoint = \"https://gcp-us-central1.turbopuffer.com\"\napi_key = \"\"\nnamespace = \"hev-traces\"\n"
	os.WriteFile(path, []byte(hosted), 0o600)
	cfg, _ := LoadConfig()
	w, err := WriteLocalConfig(cfg.Local, testKey)
	if err == nil || w.Changed {
		t.Fatalf("hosted config was repointed: %+v err=%v", w, err)
	}
	if !strings.Contains(err.Error(), "HEV_CONFIG") {
		t.Fatalf("error names no way out: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != hosted {
		t.Fatalf("hosted config modified:\n%s", got)
	}
}

func TestWriteLocalConfigKeepsWhatItDoesNotOwn(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("[layer]\nendpoint = \"http://127.0.0.1:7000\"\nnamespace = \"mine\"\n\n[capture]\nscan_interval = \"1m\"\n\n[projects]\ndeny = [\"~/secret\"]\n"), 0o600)
	cfg, _ := LoadConfig()
	if _, err := WriteLocalConfig(cfg.Local, testKey); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerNamespace != "mine" || cfg.ScanInterval.String() != "1m0s" || len(cfg.ProjectDeny) != 1 {
		t.Fatalf("lost user config: ns=%s interval=%s deny=%v", cfg.LayerNamespace, cfg.ScanInterval, cfg.ProjectDeny)
	}
	if cfg.LayerEndpoint != "http://127.0.0.1:8080" {
		t.Fatalf("endpoint = %s", cfg.LayerEndpoint)
	}
}

func TestConfigIsReplacedAtomicallyAndStaysPrivate(t *testing.T) {
	path := sandboxConfig(t)
	cfg, _ := LoadConfig()
	if _, err := WriteLocalConfig(cfg.Local, testKey); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v err = %v", info.Mode().Perm(), err)
	}
	left, _ := os.ReadDir(filepath.Dir(path))
	if len(left) != 1 {
		t.Fatalf("temp file left behind: %v", left)
	}
}

// A config the Postgres-era `hev up` wrote moves to Turbopuffer: the store and
// the placeholder key are replaced, the lexical namespace name goes back to the
// default, and the write says so, because the index state describes the old
// archive.
func TestWriteLocalConfigMovesThePostgresEraConfig(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	old := "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"local\"\nnamespace = \"hev-traces-local\"\nstore = \"pgvector\"\n\n[local]\nimage = \"hevlayer/layer-gateway:edge\"\nport = 8080\nproject = \"hev-kit\"\nserve_port = 8099\n"
	os.WriteFile(path, []byte(old), 0o600)
	if LocalAPIKey() != "" {
		t.Fatal("the placeholder key was taken for a real one")
	}
	cfg, _ := LoadConfig()
	w, err := WriteLocalConfig(cfg.Local, testKey)
	if err != nil || !w.Changed || !w.Migrated {
		t.Fatalf("write = %+v err=%v", w, err)
	}
	cfg, _ = LoadConfig()
	if cfg.LayerStore != "turbopuffer" || cfg.LayerNamespace != "hev-traces" || cfg.LayerAPIKey != testKey {
		t.Fatalf("config = store %s ns %s", cfg.LayerStore, cfg.LayerNamespace)
	}
	if w, _ := WriteLocalConfig(cfg.Local, testKey); w.Changed || w.Migrated {
		t.Fatalf("second write = %+v", w)
	}
}
