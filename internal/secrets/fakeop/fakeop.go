// Package fakeop installs a fake `op` executable for tests. It never calls
// 1Password: every answer is made up, and each invocation appends one line
// (its verb and arguments, never stdin) to a log the test can count.
package fakeop

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Value is what the fake returns for every reference.
const Value = "fake-secret-value-never-log-this"

const script = `#!/bin/sh
# Fake op for kit tests. Never contacts 1Password.
printf '%s\n' "$*" >> "$FAKE_OP_CALLS"
if [ -n "$FAKE_OP_ERROR" ]; then
  echo "$FAKE_OP_ERROR" >&2
  exit 1
fi
if [ -n "$FAKE_OP_RATE_LIMIT" ]; then
  echo "[ERROR] Too many requests. daily rate limit exceeded, reset at $FAKE_OP_RATE_LIMIT" >&2
  exit 1
fi
case "$1" in
inject)
  sed 's/{{ [^}]* }}/` + Value + `/g'
  ;;
read)
  if [ "$2" = "--no-newline" ]; then printf '%s' "` + Value + `"; else printf '%s\n' "` + Value + `"; fi
  ;;
service-account)
  printf '%s' '[{"type":"token","action":"read","limit":1000,"used":10,"remaining":990,"reset":3600},{"type":"account","action":"read_write","limit":1000,"used":1000,"remaining":0,"reset":"2026-11-11T21:30:00Z"}]'
  ;;
*)
  exit 2
  ;;
esac
`

// Fake is an installed fake op.
type Fake struct {
	Bin   string // directory holding op; put it first on PATH
	Calls string // file with one line per invocation
}

// Install writes the fake into a new temporary directory, points PATH at it
// (plus /bin and /usr/bin for sh and sed) and sets FAKE_OP_CALLS. It skips the
// test on Windows, which has no sh.
func Install(t testing.TB) Fake {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake op is a shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "op"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	f := Fake{Bin: bin, Calls: filepath.Join(dir, "calls")}
	t.Setenv("FAKE_OP_CALLS", f.Calls)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
	resolved, err := exec.LookPath("op")
	if err != nil || resolved != filepath.Join(bin, "op") {
		t.Fatal("fixture could execute real provider CLI")
	}
	return f
}

// CallLines returns the recorded invocations.
func (f Fake) CallLines(t testing.TB) []string {
	t.Helper()
	b, err := os.ReadFile(f.Calls)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// Count returns how many times op ran.
func (f Fake) Count(t testing.TB) int { return len(f.CallLines(t)) }
