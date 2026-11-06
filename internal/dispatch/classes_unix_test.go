//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeLiveLaunches records tmux sessions in a file so launched runs stay live
// until end removes them. No supervisor or harness runs.
func fakeLiveLaunches(t *testing.T) func(string) {
	t.Helper()
	bin := t.TempDir()
	state := filepath.Join(bin, "sessions")
	script := `#!/bin/sh
while [ $# -gt 0 ]; do
  case "$1" in
    new-session) while [ $# -gt 0 ]; do [ "$1" = -s ] && echo "$2" >> "$FAKE_TMUX_STATE"; shift; done; exit 0 ;;
    list-sessions) cat "$FAKE_TMUX_STATE" 2>/dev/null; exit 0 ;;
  esac
  shift
done
exit 0
`
	if e := os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("FAKE_TMUX_STATE", state)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func(name string) {
		b, _ := os.ReadFile(state)
		var keep []string
		for _, s := range strings.Fields(string(b)) {
			if s != "d-"+name {
				keep = append(keep, s+"\n")
			}
		}
		if e := os.WriteFile(state, []byte(strings.Join(keep, "")), 0600); e != nil {
			t.Fatal(e)
		}
		if e := WriteJSON(filepath.Join(Root(), name, "result.json"), Result{ExitCode: 0, End: timeNowUTC()}); e != nil {
			t.Fatal(e)
		}
	}
}
