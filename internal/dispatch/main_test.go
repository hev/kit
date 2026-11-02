package dispatch

import (
	"os"
	"testing"
)

// Even tests without their own HOME must never discover operator process records.
func TestMain(m *testing.M) {
	// Helpers inherit the parent test HOME and may exit or be killed without
	// returning from m.Run. Do not allocate resources they cannot clean up.
	if os.Getenv("DISPATCH_SIGNAL_HELPER") == "1" {
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", "kit-dispatch-tests-")
	if err != nil {
		panic(err)
	}
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(key, dir); err != nil {
			panic(err)
		}
	}
	cleanup := isolateTestProcesses()
	code := m.Run()
	cleanup()
	os.RemoveAll(dir)
	os.Exit(code)
}
