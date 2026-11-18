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
		// Detached children (the validation-notification launcher) are not
		// waited for, so readEnvDump polls for the file. Write a temporary file
		// and rename it into place: the poller must never see the dump between
		// creation and write, an empty environment without PATH.
		name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		f, err := os.CreateTemp(dir, name+".*.tmp")
		if err != nil {
			os.Exit(1)
		}
		_, err = f.WriteString(strings.Join(os.Environ(), "\n"))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Rename(f.Name(), filepath.Join(dir, name+".env"))
		}
		if err != nil {
			_ = os.Remove(f.Name())
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
