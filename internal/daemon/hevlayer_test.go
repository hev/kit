package daemon

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layerenv"
)

func TestHEVLayerConfigCompatibility(t *testing.T) {
	for _, prefix := range []string{"HEVLAYER_", "LAYER_"} {
		t.Run(prefix, func(t *testing.T) {
			for _, name := range layerenv.Names() {
				t.Setenv(name, "")
			}
			t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
			for name, value := range map[string]string{"API_KEY": "fixture-key", "ENDPOINT": "https://gateway.example", "NAMESPACE": "archive", "STORE": "hosted", "EMBED_MODEL": "fixture-model"} {
				t.Setenv(prefix+name, value)
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LayerAPIKey != "fixture-key" || cfg.LayerEndpoint != "https://gateway.example" || cfg.LayerNamespace != "archive" || cfg.LayerStore != "hosted" {
				t.Fatal("effective config ignored environment")
			}
			key, err := LayerKey()
			if err != nil || key != "fixture-key" {
				t.Fatalf("key resolver failed: %v", err)
			}
			t.Setenv(prefix+"ENDPOINT", "")
			cfg, err = LoadConfig()
			if err != nil || cfg.LayerEndpoint != layerenv.HostedEndpoint {
				t.Fatalf("missing hosted default: %v", err)
			}
			t.Setenv("HEVLAYER_API_KEY", "other-key")
			cfg, err = LoadConfig()
			if err != nil || cfg.LayerAPIKey != "other-key" {
				t.Fatalf("canonical key must win without conflict: %v", err)
			}
			key, err = LayerKey()
			if err != nil || key != "other-key" {
				t.Fatalf("canonical resolver must win: %v", err)
			}

		})
	}
}

func TestHEVLayerKeyRequiresExplicitCredentialSource(t *testing.T) {
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
	}
	// Even with op available, an implicit provider-named vault item must never
	// supply a bearer to the new hosted default.
	if _, err := LayerKey(); err == nil || !strings.Contains(err.Error(), "HEVLAYER_API_KEY_OP_REF") {
		t.Fatalf("implicit credential lookup allowed: %v", err)
	}
}
