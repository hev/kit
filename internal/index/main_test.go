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
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
