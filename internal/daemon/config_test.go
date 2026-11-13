package daemon

import (
	"github.com/hev/kit/internal/skillsources"
	"github.com/hev/kit/internal/update"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLayerConfigAndEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	t.Setenv("HEVLAYER_ENDPOINT", "")
	t.Setenv("HEVLAYER_API_KEY", "env-key")
	t.Setenv("HEVLAYER_NAMESPACE", "")
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

func TestDefaultConfigNamesNoLayerAndNoLegacyPolicy(t *testing.T) {
	text := DefaultConfigText()
	// The stock config must not say "Layer": with no key it is a files
	// machine, and an endpoint or namespace would make kit refuse files.
	if !strings.Contains(text, "# [layer]") || strings.Contains(text, "\n[layer]") || strings.Contains(text, "[[buckets]]") || strings.Contains(text, "[redaction]") {
		t.Fatalf("unexpected default config:\n%s", text)
	}
}

func TestLegacyS3OnlyConfigNamesMigration(t *testing.T) {
	err := validateLayerTarget(&Config{ActiveBucket: "old", Buckets: []BucketConfig{{Name: "old"}}})
	if err == nil || !strings.Contains(err.Error(), "hev init") || !strings.Contains(err.Error(), "hev migrate") {
		t.Fatalf("error = %v", err)
	}
}

func TestNamespaceEnvironmentOverridesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	t.Setenv("HEVLAYER_NAMESPACE", "host-archive")
	if err := os.WriteFile(path, []byte("[layer]\nnamespace = \"file-archive\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerNamespace != "host-archive" {
		t.Fatalf("namespace = %q", cfg.LayerNamespace)
	}
}

func TestSetActiveBucketPreservesSkillSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	text := "[[buckets]]\nname = 'example'\n[skills]\nrole = 'host'\nmachine = 'worker'\n[[skills.sources]]\nurl = 'https://example.com/team/skills.git'\nref = 'refs/heads/main'\nsubdir = 'skills'\n"
	if e := os.WriteFile(path, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := SetActiveBucket("example"); e != nil {
		t.Fatal(e)
	}
	cfg, e := skillsources.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Role != "host" || cfg.Machine != "worker" || len(cfg.Sources) != 1 || cfg.Sources[0].Ref != "refs/heads/main" || cfg.Sources[0].Subdir != "skills" {
		t.Fatalf("lost skill sources: %+v", cfg)
	}
}

func TestSetActiveBucketPreservesUpdateSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	text := "[[buckets]]\nname = 'example'\n[update]\nchannel = 'edge'\nauto = true\ninterval = '2h'\npost_update = ['systemctl --user restart wrapper']\n"
	if e := os.WriteFile(path, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := SetActiveBucket("example"); e != nil {
		t.Fatal(e)
	}
	cfg, e := update.LoadConfig(path)
	if e != nil || cfg.Channel != "edge" || !cfg.Auto || cfg.Interval != "2h" || len(cfg.PostUpdate) != 1 {
		t.Fatalf("lost [update]: %+v %v", cfg, e)
	}
}
