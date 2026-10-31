//go:build windows

package redact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsRedactionPersistsIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "config.toml")
	first, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || second == nil || string(before) != string(after) {
		t.Fatal("redaction salt changed")
	}
	t.Setenv("HEV_CONFIG", path)
	release, err := LockArchive()
	if err != nil {
		t.Fatal(err)
	}
	release()
}
