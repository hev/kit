package filearchive

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain points HEV_CONFIG at a scratch file: writes load the capture
// scrubber, which creates its salt in the config, and a test must never touch
// the operator's ~/.hev/config.toml.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "filearchive-config-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
