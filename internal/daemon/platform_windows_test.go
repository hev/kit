//go:build windows

package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsDaemonValidatesConfig(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	path := filepath.Join(home, "config.toml")
	t.Setenv("HEV_CONFIG", path)
	t.Setenv("LAYER_API_KEY", "")
	t.Setenv("LAYER_NAMESPACE", "")
	t.Setenv("LAYER_ENDPOINT", "")
	if err := os.WriteFile(path, []byte("[layer]\nendpoint='http://127.0.0.1:1'\napi_key='test'\nnamespace='test'\n[capture]\nscan_interval='-1s'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := Run(ctx); err == nil || !strings.Contains(err.Error(), "scan interval must be positive") {
		t.Fatalf("config validation: %v", err)
	}
}
