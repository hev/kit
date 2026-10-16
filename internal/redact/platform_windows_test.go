//go:build windows

package redact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsLoadFailsBeforeCreatingConfigDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	path := filepath.Join(dir, "config.toml")
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "UnsupportedCapability") {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("created config directory: %v", err)
	}
	t.Setenv("HEV_CONFIG", path)
	if _, err := LockArchive(); err == nil || !strings.Contains(err.Error(), "UnsupportedCapability") {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("created archive lock directory: %v", err)
	}
}
