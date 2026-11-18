package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/version"
)

func sandboxConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", path)
	for _, env := range []string{"HEVLAYER_API_KEY_REF", "HEVLAYER_API_KEY_OP_REF", "LAYER_API_KEY_OP_REF", "LAYER_API_KEY", "TPUF_API_KEY", "HEVLAYER_ENDPOINT", "HEVLAYER_API_KEY", "HEVLAYER_NAMESPACE", "HEVLAYER_STORE", "HEV_LOCAL_IMAGE", "HEV_LOCAL_PROJECT", "HEV_LOCAL_PORT", "HEV_LOCAL_SERVE_PORT", "HEV_LOCAL_KIT_IMAGE"} {
		t.Setenv(env, "")
	}
	return path
}

const testKey = "tpuf_test_key"

// The pin, the port and the project are each one value in the file.
func TestLocalBlockIsReadBack(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("[local]\nimage = \"hevlayer/layer-gateway:v0.6.1\"\nport = 9191\nproject = \"mine\"\nkit_image = \"registry.example/kit:mine\"\n"), 0o600)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := LocalConfig{Managed: true, Image: "hevlayer/layer-gateway:v0.6.1", Port: 9191, Project: "mine", KitImage: "registry.example/kit:mine", ServePort: DefaultLocalServePort}
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

// With no key, `hev up` writes a config that names no key and no store, and
// drops what the retired local Docker lane wrote, keeping everything else.
func TestWriteFilesConfigDropsTheRetiredLane(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	old := "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"local\"\nnamespace = \"mine\"\nstore = \"" + RetiredStore + "\"\n\n[local]\nimage = \"hevlayer/layer-gateway:edge\"\nembed_image = \"hevlayer/layer-embed:0.7.3\"\nport = 8080\nproject = \"hev-kit\"\nserve_port = 8099\n\n[capture]\nscan_interval = \"2m\"\n"
	os.WriteFile(path, []byte(old), 0o600)
	cfg, _ := LoadConfig()
	w, err := WriteFilesConfig(cfg.Local)
	if err != nil || !w.Changed || w.Migrated {
		t.Fatalf("write = %+v err=%v", w, err)
	}
	raw, _ := os.ReadFile(path)
	for _, gone := range []string{RetiredStore, "api_key", "127.0.0.1", "embed_image"} {
		if strings.Contains(string(raw), gone) {
			t.Fatalf("%s kept:\n%s", gone, raw)
		}
	}
	cfg, _ = LoadConfig()
	if cfg.LayerStore != "" || cfg.LayerAPIKey != "" || cfg.LayerNamespace != "mine" || !cfg.Local.Managed || cfg.ScanInterval.String() != "2m0s" {
		t.Fatalf("config = %+v", cfg)
	}
	// The retired lane's namespace now names the file archive under
	// [archive]; under [layer] it would say hev layer.
	if !strings.Contains(string(raw), "[archive]") || strings.Contains(strings.SplitN(string(raw), "[archive]", 2)[0], "namespace") {
		t.Fatalf("namespace not moved to [archive]:\n%s", raw)
	}
	target, err := OpenTarget(cfg, "")
	if err != nil || target.Files == nil || target.Retired || filepath.Base(target.Files.Dir()) != "mine" {
		t.Fatalf("target = %+v, %v", target, err)
	}
	if w, _ := WriteFilesConfig(cfg.Local); w.Changed {
		t.Fatalf("second write = %+v", w)
	}
}

// An upgrade moves the images an earlier `hev up` recorded as its defaults;
// an image the user chose stays, and an env override still wins.
func TestLoadConfigMovesRecordedDefaultImages(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	write := func(image, kitImage string) {
		body := "[layer]\nendpoint = \"http://127.0.0.1:8080\"\nstore = \"turbopuffer\"\n\n[local]\nimage = \"" + image + "\"\nkit_image = \"" + kitImage + "\"\n"
		os.WriteFile(path, []byte(body), 0o600)
	}

	for _, prior := range []string{"hevlayer/layer-gateway:edge", "hevlayer/layer-gateway:0.6.0", "hevlayer/layer-gateway:0.7.0", "hevlayer/layer-gateway:0.7.1", "hevlayer/layer-gateway:0.7.2"} {
		write(prior, "hevlayer/kit:0.1.0")
		cfg, _ := LoadConfig()
		if cfg.Local.Image != DefaultLocalImage || cfg.Local.KitImage != version.KitImage() {
			t.Fatalf("recorded defaults kept: %s %s", cfg.Local.Image, cfg.Local.KitImage)
		}
	}

	write("registry.example/layer-gateway:mine", "registry.example/kit:mine")
	cfg, _ := LoadConfig()
	if cfg.Local.Image != "registry.example/layer-gateway:mine" || cfg.Local.KitImage != "registry.example/kit:mine" {
		t.Fatalf("chosen images replaced: %s %s", cfg.Local.Image, cfg.Local.KitImage)
	}

	write("hevlayer/layer-gateway:edge", "hevlayer/kit:0.1.0")
	t.Setenv("HEV_LOCAL_IMAGE", "hevlayer/layer-gateway:edge")
	t.Setenv("HEV_LOCAL_KIT_IMAGE", "hevlayer/kit:0.1.0")
	cfg, _ = LoadConfig()
	if cfg.Local.Image != "hevlayer/layer-gateway:edge" || cfg.Local.KitImage != "hevlayer/kit:0.1.0" {
		t.Fatalf("env override lost: %s %s", cfg.Local.Image, cfg.Local.KitImage)
	}
}

// A config the removed BYO Turbopuffer lane wrote holds the provider key as
// [layer] api_key in front of a loopback gateway. Moving it to files drops
// that key with the store and the endpoint: left there it would read as a hev
// layer key.
func TestWriteFilesConfigDropsTheBYOTurbopufferLane(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	old := "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"" + testKey + "\"\nnamespace = \"hev-traces\"\nstore = \"turbopuffer\"\n\n[local]\nport = 8080\n"
	os.WriteFile(path, []byte(old), 0o600)
	cfg, _ := LoadConfig()
	if _, err := WriteFilesConfig(cfg.Local); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	for _, gone := range []string{"turbopuffer", testKey, "127.0.0.1"} {
		if strings.Contains(string(raw), gone) {
			t.Fatalf("%s kept:\n%s", gone, raw)
		}
	}
	cfg, _ = LoadConfig()
	if target, err := OpenTarget(cfg, ""); err != nil || target.Files == nil {
		t.Fatalf("target = %+v, %v", target, err)
	}
}

// A files config passes the daemon's target check with no endpoint or key.
func TestValidateLayerTargetAcceptsFiles(t *testing.T) {
	if err := validateLayerTarget(&Config{LayerStore: "files"}); err != nil {
		t.Fatal(err)
	}
}
