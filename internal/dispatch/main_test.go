package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Even tests without their own HOME must never discover operator process records.
func TestMain(m *testing.M) {
	// Copies of this binary stand in for plugins, harnesses and supervisors
	// in env isolation tests: they record their environment and exit.
	if dir := os.Getenv("DISPATCH_ENV_DUMP"); dir != "" {
		name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		if err := os.WriteFile(filepath.Join(dir, name+".env"), []byte(strings.Join(os.Environ(), "\n")), 0600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	// Helpers inherit the parent test HOME and may exit or be killed without
	// returning from m.Run. Do not allocate resources they cannot clean up.
	if os.Getenv("DISPATCH_SIGNAL_HELPER") == "1" || os.Getenv("ORCH_PROCESS_HELPER") == "1" {
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
