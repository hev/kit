package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/version"
)

func sandboxConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", path)
	for _, env := range []string{"HEVLAYER_ENDPOINT", "HEVLAYER_API_KEY", "HEVLAYER_NAMESPACE", "HEVLAYER_STORE", "HEV_LOCAL_IMAGE", "HEV_LOCAL_PROJECT", "HEV_LOCAL_PORT", "HEV_LOCAL_SERVE_PORT", "HEV_LOCAL_KIT_IMAGE"} {
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
	if w, err := WriteLocalConfig(cfg.Local, layer.StoreTurbopuffer, testKey); err != nil || !w.Changed || w.Migrated {
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
	if w, err := WriteLocalConfig(cfg.Local, layer.StoreTurbopuffer, testKey); err != nil || w.Changed {
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

func TestWriteLocalConfigRefusesAHostedConfig(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	hosted := "[layer]\nendpoint = \"https://gcp-us-central1.turbopuffer.com\"\napi_key = \"\"\nnamespace = \"hev-traces\"\n"
	os.WriteFile(path, []byte(hosted), 0o600)
	cfg, _ := LoadConfig()
	w, err := WriteLocalConfig(cfg.Local, layer.StoreTurbopuffer, testKey)
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
	os.WriteFile(path, []byte("[layer]\nendpoint = \"http://127.0.0.1:7000\"\nnamespace = \"mine\"\n\n[capture]\nscan_interval = \"1m\"\n"), 0o600)
	cfg, _ := LoadConfig()
	if _, err := WriteLocalConfig(cfg.Local, layer.StoreTurbopuffer, testKey); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerNamespace != "mine" || cfg.ScanInterval.String() != "1m0s" {
		t.Fatalf("lost user config: ns=%s interval=%s", cfg.LayerNamespace, cfg.ScanInterval)
	}
	if cfg.LayerEndpoint != "http://127.0.0.1:8080" {
		t.Fatalf("endpoint = %s", cfg.LayerEndpoint)
	}
}

func TestConfigIsReplacedAtomicallyAndStaysPrivate(t *testing.T) {
	path := sandboxConfig(t)
	cfg, _ := LoadConfig()
	if _, err := WriteLocalConfig(cfg.Local, layer.StoreTurbopuffer, testKey); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v err = %v", info.Mode().Perm(), err)
	}
	left, _ := os.ReadDir(filepath.Dir(path))
	if len(left) != 1 {
		t.Fatalf("temp file left behind: %v", left)
	}
}

// With no key, `hev up` writes a config that names no key and no store, and
// drops what the retired local Docker lane wrote, keeping everything else.
func TestWriteFilesConfigDropsTheRetiredLane(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	old := "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"local\"\nnamespace = \"mine\"\nstore = \"" + RetiredStore + "\"\n\n[local]\nimage = \"hevlayer/layer-gateway:edge\"\nembed_image = \"hevlayer/layer-embed:0.7.3\"\nport = 8080\nproject = \"hev-kit\"\nserve_port = 8099\n\n[capture]\nscan_interval = \"2m\"\n"
	os.WriteFile(path, []byte(old), 0o600)
	if LocalAPIKey() != "" {
		t.Fatal("the placeholder key was taken for a real one")
	}
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
	target, err := OpenTarget(cfg, "")
	if err != nil || target.Files == nil || target.Retired {
		t.Fatalf("target = %+v, %v", target, err)
	}
	if w, _ := WriteFilesConfig(cfg.Local); w.Changed {
		t.Fatalf("second write = %+v", w)
	}
}

// Moving the Turbopuffer lane to another store is a new archive.
func TestWriteLocalConfigBetweenStoresIsAMigration(t *testing.T) {
	sandboxConfig(t)
	cfg, _ := LoadConfig()
	if w, _ := WriteLocalConfig(cfg.Local, layer.StoreTurbopuffer, testKey); !w.Changed || w.Migrated {
		t.Fatalf("first write = %+v", w)
	}
	if LocalAPIKey() != testKey || LocalStore() != layer.StoreTurbopuffer {
		t.Fatalf("key %q store %q", LocalAPIKey(), LocalStore())
	}
	if w, _ := WriteLocalConfig(cfg.Local, "other", testKey); !w.Migrated {
		t.Fatalf("to another store = %+v", w)
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

func TestHEVLayerExplicitMoveFromHosted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, "config.toml")
	t.Setenv("HEV_CONFIG", path)
	if err := os.WriteFile(path, []byte("[layer]\nendpoint='https://gateway.example'\napi_key='hosted-key'\nnamespace='archive'\nstore='hosted'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckLocalConfig(); err == nil {
		t.Fatal("implicit hosted move allowed")
	}
	if err := CheckLocalConfig(true); err != nil {
		t.Fatal(err)
	}
	w, err := WriteLocalConfig(LocalConfig{Port: 8080, ServePort: 8099, Project: "test"}, layer.StoreTurbopuffer, testKey, true)
	if err != nil || !w.Migrated {
		t.Fatalf("explicit hosted move: %+v %v", w, err)
	}
	cfg, err := LoadConfig()
	if err != nil || cfg.LayerStore != layer.StoreTurbopuffer || cfg.LayerEndpoint != "http://127.0.0.1:8080" {
		t.Fatalf("new target: %v", err)
	}
}
