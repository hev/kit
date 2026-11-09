//go:build !windows

package peers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Under launchd, systemd or ssh without LANG the environment has no locale.
// The engine must still read tmux's tab-separated pane list. The test uses
// its own tmux server under a temporary TMUX_TMPDIR and an explicit socket,
// so it can never touch the user's server.
func TestListPanesWithEmptyEnvironment(t *testing.T) {
	isolated := isolatedTmux(t, "locale-probe", "sleep 60")
	// Empty the environment for the engine's own tmux calls.
	saved := os.Environ()
	os.Clearenv()
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range saved {
			if k, v, ok := strings.Cut(kv, "="); ok {
				os.Setenv(k, v)
			}
		}
	})
	for _, kv := range isolated {
		k, v, _ := strings.Cut(kv, "=")
		os.Setenv(k, v)
	}
	if got := os.Getenv("LANG") + os.Getenv("LC_ALL") + os.Getenv("LC_CTYPE"); got != "" {
		t.Fatal("locale still set", got)
	}
	e := New()
	e.State = t.TempDir()
	peers, err := e.list(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range peers {
		if p.Session == "locale-probe" && p.Pane != "" {
			return
		}
	}
	t.Fatalf("pane not listed without a locale: %+v", peers)
}

// isolatedTmux starts a tmux server under a temporary TMUX_TMPDIR, with one
// session running cmd, and returns the environment that reaches it. It never
// touches the user's server.
func isolatedTmux(t *testing.T, session, cmd string) []string {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	tmp, err := os.MkdirTemp("/tmp", "peers-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	sockDir := filepath.Join(tmp, "tmux-"+strconv.Itoa(os.Getuid()))
	if err := os.MkdirAll(sockDir, 0700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(sockDir, "default")
	isolated := []string{"HOME=" + t.TempDir(), "PATH=" + filepath.Dir(tmux) + ":/usr/bin:/bin", "TMUX_TMPDIR=" + tmp}
	start := exec.Command(tmux, "-S", socket, "-f", "/dev/null", "new-session", "-d", "-s", session, cmd)
	start.Env = isolated
	if out, err := start.CombinedOutput(); err != nil {
		t.Skip("cannot start an isolated tmux server: ", err, string(out))
	}
	t.Cleanup(func() {
		kill := exec.Command(tmux, "-S", socket, "kill-server")
		kill.Env = isolated
		_ = kill.Run()
	})
	return isolated
}
