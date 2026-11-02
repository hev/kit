package dispatch

import (
	"os"
	"path/filepath"
	"testing"
)

// Even tests without their own HOME must never discover operator process records.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "kit-dispatch-tests-")
	if err != nil {
		panic(err)
	}
	// The signal helper is a child of this suite and needs its parent
	// test's temporary HOME to find the supervisor record.
	if os.Getenv("DISPATCH_SIGNAL_HELPER") != "1" {
		for _, key := range []string{"HOME", "USERPROFILE"} {
			if err := os.Setenv(key, dir); err != nil {
				panic(err)
			}
		}
	}
	cleanup := isolateTestProcesses(filepath.Join(dir, "tmux.sock"))
	code := m.Run()
	cleanup()
	os.RemoveAll(dir)
	os.Exit(code)
}
