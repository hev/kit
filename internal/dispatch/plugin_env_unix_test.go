//go:build !windows

package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func allowChildProbes(*testing.T) {}

// The worker supervisor starts in a tmux session. A server that already
// carries the secret must not pass it on, nor may a host env entry.
func TestPluginOnlyEnvUnix(t *testing.T) {
	_, dump := testPluginOnlyEnv(t)
	socket, cleanup, e := newTestTmuxSocket()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(cleanup)
	old := tmuxCommand
	tmuxCommand = func(args ...string) *exec.Cmd {
		c := exec.Command("tmux", "-S", socket)
		c.Args = append(c.Args, args...)
		return c
	}
	t.Cleanup(func() { _ = tmuxCommand("kill-server").Run(); tmuxCommand = old })
	server := tmuxCommand("new-session", "-d", "-s", "keep", "sleep 60")
	server.Env = os.Environ()
	if out, e := server.CombinedOutput(); e != nil {
		t.Fatalf("tmux server: %v: %s", e, out)
	}
	dir := filepath.Join(Root(), "env-supervisor")
	if e := os.MkdirAll(filepath.Join(dir, "wt"), 0700); e != nil {
		t.Fatal(e)
	}
	m := Meta{Start: time.Now().UTC(), LaunchResolved: true, LaunchEnv: []string{pluginSecret, "DISPATCH_ENV_DUMP"}, Order: Order{Name: "env-supervisor", Repo: "app"}}
	if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
		t.Fatal(e)
	}
	if e := launch(dir, "env-supervisor"); e != nil {
		t.Fatal(e)
	}
	env := readEnvDump(t, dump, filepath.Base(os.Args[0]))
	if !hasEnv(env, "DISPATCH_ENV_DUMP") {
		t.Fatal("supervisor environment not forwarded")
	}
	if hasEnv(env, pluginSecret) || hasEnv(env, otherSecret) {
		t.Fatal("worker supervisor received a plugin-only name")
	}
}
