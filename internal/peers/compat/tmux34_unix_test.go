//go:build !windows

package compat

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A pane whose title (or cwd) holds the four characters \037 must survive
// tmux 3.4's escaped output: before the q form it split into an extra field
// and panes() dropped the pane. The fake tmux replays real 3.4 output.
func TestPanesKeepsLiteralEscapeOnTmux34(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	must(t, os.MkdirAll(bin, 0o755))
	must(t, os.WriteFile(filepath.Join(home, "plain"), []byte(tmux34Plain+"\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(home, "quoted"), []byte(tmux34Quoted+"\n"), 0o644))
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"" + home + "/calls\"\n" +
		"case \"$*\" in *list-panes*'#{q:'*) cat \"" + home + "/quoted\";; *list-panes*) cat \"" + home + "/plain\";; esac\n"
	must(t, os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0o755))
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HEV_PEERS_STATE", filepath.Join(home, "state"))

	p := newP(strings.NewReader(""), io.Discard, io.Discard)
	// newP prepends installed tool paths; keep this fixture's tmux first.
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	for round := 0; round < 2; round++ {
		var found *Row
		for _, r := range p.panes(true) {
			if r.ID == "%0" {
				found = r
			}
		}
		if found == nil {
			t.Fatalf("round %d: pane %%0 dropped", round)
		}
		if found.Title != `literal\037title` || found.Cwd != `/tmp/w\037d $x\y` || found.Session != "alpha" ||
			found.PID != 1313881 || found.TmuxIdentity != "1313880:$0:1794271619" {
			t.Fatalf("round %d: row %+v", round, *found)
		}
	}
	calls, _ := os.ReadFile(filepath.Join(home, "calls"))
	// The first listing finds no separator byte and asks again in q form;
	// later listings go straight to the q form.
	if n := strings.Count(string(calls), "list-panes"); n != 3 {
		t.Fatalf("list-panes calls = %d, want 3:\n%s", n, calls)
	}
}
