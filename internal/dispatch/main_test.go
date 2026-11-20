package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Even tests without their own HOME must never discover operator process records.
func TestMain(m *testing.M) {
	// Copies of this binary stand in for plugins, harnesses and supervisors
	// in env isolation tests: they record their arguments and environment and
	// exit.
	if dir := os.Getenv("DISPATCH_ENV_DUMP"); dir != "" {
		name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		// A harness diagnostic that echoes a variable's value, split across
		// stdout and stderr in separate writes, exercises output redaction.
		if echo := os.Getenv("DISPATCH_ECHO_ENV"); echo != "" {
			v := os.Getenv(echo)
			half := len(v) / 2
			_, _ = os.Stdout.WriteString("diagnostic: key=" + v[:half])
			time.Sleep(50 * time.Millisecond)
			_, _ = os.Stderr.WriteString(v[half:] + " end\n")
		}
		// Arguments first: tests wait on the .env file, then read .args.
		if err := os.WriteFile(filepath.Join(dir, name+".args"), []byte(strings.Join(os.Args[1:], "\n")), 0600); err != nil {
			os.Exit(1)
		}
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
	// Notices resolve requested_by through /v1/whoami when signed in; tests
	// sign in explicitly (HEV_CONFIG) and never through the operator's key.
	_ = os.Unsetenv("HEV_API_KEY")
	// A worker's HEV_REQUESTED_BY names who its child runs are for.
	_ = os.Unsetenv("HEV_REQUESTED_BY")
	cleanup := isolateTestProcesses()
	code := m.Run()
	cleanup()
	os.RemoveAll(dir)
	os.Exit(code)
}
