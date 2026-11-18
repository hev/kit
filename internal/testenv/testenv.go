// Package testenv holds test helpers for the hosts kit runs on, macOS
// included. Import it from tests and test fixtures only.
package testenv

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// ShortDir makes a new private directory short enough to hold Unix sockets
// and tmux socket directories. A socket path must fit sockaddr_un (104 bytes
// on macOS, 108 on Linux), and on macOS TMPDIR is a long /var/folders path,
// so on Unix the directory goes under /tmp. On Windows it goes under the
// usual temporary directory. The caller removes it.
func ShortDir(prefix string) (string, error) {
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = ""
	}
	return os.MkdirTemp(base, prefix)
}

// SocketDir is ShortDir for one test: a new directory for sockets, removed
// when the test ends.
func SocketDir(t testing.TB) string {
	t.Helper()
	dir, err := ShortDir("kit")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// PrimeArg is the only argument PrimeExec passes. A fixture script that
// would block or record the call should exit early when it sees it.
const PrimeArg = "kit-test-prime"

// PrimeExec runs each newly written executable once, with PrimeArg, stdin
// from /dev/null and env added to the test's environment, and ignores how it
// exits. macOS checks a new executable file the first time it runs, and
// serialises those checks across processes, at roughly 100-200 ms a file. A
// test that holds handlers to a deadline of a few hundred milliseconds must
// not count that one-time cost; installed handlers pay it once, not per call.
func PrimeExec(t testing.TB, env []string, paths ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, p := range paths {
		c := exec.CommandContext(ctx, p, PrimeArg)
		c.Env = append(os.Environ(), env...)
		_ = c.Run()
		if ctx.Err() != nil {
			t.Fatalf("priming %s did not finish: does it exit on %s?", p, PrimeArg)
		}
	}
}
