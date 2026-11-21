//go:build windows

package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsMetaBatchHandler(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("HEV_HOOKS", "")
	script := "@echo off\r\nif not \"%~1\"==\"admission-check\" exit /b 2\r\nif not \"%HEV_HOOK_HARNESS%\"==\"kit\" exit /b 3\r\necho {\"decision\":\"block\",\"reason\":\"fixture denial\"}\r\n"
	if err := os.WriteFile(filepath.Join(home, "hev-hook-policy.cmd"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	r := Meta(context.Background(), "admission-check", map[string]any{"name": "fixture"}, "")
	if r.Decision != Block || r.Reason != "fixture denial" {
		t.Fatalf("batch handler response = %+v", r)
	}
}
