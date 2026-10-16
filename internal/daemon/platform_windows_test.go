//go:build windows

package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsDaemonFailsBeforeOpeningConfigOrLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "not-present", "secret-config.toml"))
	if err := Run(context.Background()); err == nil || !strings.Contains(err.Error(), "UnsupportedCapability") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".hev")); !os.IsNotExist(err) {
		t.Fatalf("created daemon directory: %v", err)
	}
}
