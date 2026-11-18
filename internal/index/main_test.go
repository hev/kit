package index

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests must never create a fingerprint identity in the operator's config.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "kit-index-tests-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
	// Capture derives a session's starting context from home: keep the
	// operator's instruction files and skills out of every row.
	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0o755)
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
