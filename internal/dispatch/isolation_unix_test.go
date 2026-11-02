//go:build !windows

package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var testSocket string

func isolateTestProcesses(socket string) func() {
	testSocket = socket
	// Explicit -S wins even if a test changes HOME, TMUX or TMUX_TMPDIR.
	// PATH remains mockable by individual tests.
	tmuxCommand = func(args ...string) *exec.Cmd {
		for _, arg := range args {
			if arg == "-S" || arg == "-L" {
				panic("test attempted to override isolated tmux socket")
			}
		}
		c := exec.Command("tmux", "-S", socket)
		c.Args = append(c.Args, args...)
		return c
	}
	return func() { _ = tmuxCommand("kill-server").Run() }
}

func TestTmuxIsolationGuard(t *testing.T) {
	// A hostile inherited socket must never redirect any command to another server.
	t.Setenv("TMUX", "/operator/tmux/socket,1,0")
	t.Setenv("TMUX_TMPDIR", "/operator/tmux")
	for _, args := range [][]string{
		{"list-sessions", "-F", "#S"},
		{"new-session", "-d", "-s", "d-example"},
		{"kill-session", "-t", "d-example"},
		{"kill-server"},
	} {
		c := tmuxCommand(args...)
		if len(c.Args) < 4 || c.Args[1] != "-S" || c.Args[2] != testSocket {
			t.Fatalf("unisolated tmux command: %q", c.Args)
		}
	}
	for _, flag := range []string{"-S", "-L"} {
		t.Run(flag, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("socket override was allowed")
				}
			}()
			tmuxCommand(flag, "foreign", "kill-server")
		})
	}
}

func TestTmuxIsolationIgnoresForeignSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	// This second private server stands in for the operator's server. It is
	// created by this test; no command here ever accesses the real default server.
	foreign := filepath.Join(t.TempDir(), "foreign.sock")
	run := func(socket string, args ...string) {
		t.Helper()
		c := exec.Command("tmux", "-S", socket)
		c.Args = append(c.Args, args...)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v: %s", args, err, out)
		}
	}
	run(foreign, "new-session", "-d", "-s", "d-foreign", "sleep 60")
	t.Cleanup(func() { _ = exec.Command("tmux", "-S", foreign, "kill-server").Run() })
	t.Setenv("TMUX", foreign+",1,0")
	run(testSocket, "new-session", "-d", "-s", "d-owned", "sleep 60")
	t.Cleanup(func() { _ = tmuxCommand("kill-session", "-t", "=d-owned").Run() })
	names, err := liveNames()
	if err != nil || len(names) != 1 || !names["owned"] {
		t.Fatalf("foreign session leaked into inventory: %v %v", names, err)
	}
	if out, err := tmuxCommand("kill-session", "-t", "=d-owned").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	run(foreign, "has-session", "-t", "=d-foreign")
}

func TestLaunchUsesIsolatedSocket(t *testing.T) {
	bin := t.TempDir()
	capture := filepath.Join(bin, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + capture + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{LaunchResolved: true}); err != nil {
		t.Fatal(err)
	}
	if err := launch(dir, "example"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(capture)
	if err != nil || !strings.HasPrefix(string(b), "-S\n"+testSocket+"\nnew-session\n") {
		t.Fatalf("launch escaped socket: %q %v", b, err)
	}
}
