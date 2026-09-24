package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLayerConfigAndEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	t.Setenv("LAYER_ENDPOINT", "")
	t.Setenv("LAYER_API_KEY", "env-key")
	t.Setenv("LAYER_NAMESPACE", "")
	if err := os.WriteFile(path, []byte("[layer]\nendpoint = \"http://layer.test\"\napi_key = \"file-key\"\nnamespace = \"archive\"\n[capture]\nscan_interval = \"30s\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerEndpoint != "http://layer.test" || cfg.LayerAPIKey != "env-key" || cfg.LayerNamespace != "archive" || cfg.ScanInterval.String() != "30s" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestDefaultConfigHasLayerAndNoLegacyPolicy(t *testing.T) {
	text := DefaultConfigText()
	if !strings.Contains(text, "[layer]") || strings.Contains(text, "[[buckets]]") || strings.Contains(text, "[redaction]") {
		t.Fatalf("unexpected default config:\n%s", text)
	}
}

func TestLegacyS3OnlyConfigNamesMigration(t *testing.T) {
	err := validateLayerTarget(&Config{ActiveBucket: "old", Buckets: []BucketConfig{{Name: "old"}}})
	if err == nil || !strings.Contains(err.Error(), "hev init") || !strings.Contains(err.Error(), "hev migrate") {
		t.Fatalf("error = %v", err)
	}
}
