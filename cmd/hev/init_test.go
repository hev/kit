package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/daemon"
)

func TestInteractiveInitWritesLayerTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	t.Setenv("LAYER_ENDPOINT", "")
	t.Setenv("LAYER_API_KEY", "")
	t.Setenv("LAYER_NAMESPACE", "")
	var out bytes.Buffer
	if err := runInteractiveInit(strings.NewReader("http://layer.test\nsecret-key\nteam-traces\n"), &out); err != nil {
		t.Fatal(err)
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerEndpoint != "http://layer.test" || cfg.LayerAPIKey != "secret-key" || cfg.LayerNamespace != "team-traces" {
		t.Fatalf("layer config = %#v", cfg)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(b); strings.Contains(text, "buckets") || strings.Contains(text, "redaction") {
		t.Fatalf("legacy config survived:\n%s", text)
	}
}

func TestInteractiveInitRequiresCompleteTarget(t *testing.T) {
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	if err := runInteractiveInit(strings.NewReader("http://layer.test\n\nhev-traces\n"), &bytes.Buffer{}); err == nil {
		t.Fatal("expected missing API key error")
	}
}
