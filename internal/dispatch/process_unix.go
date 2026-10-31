//go:build !windows

package dispatch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func setupChild(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killTree(c *exec.Cmd) {
	if c.Process != nil {
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
func writePID(dir string) error {
	return WriteJSON(filepath.Join(dir, "supervisor.json"), map[string]int{"pid": os.Getpid()})
}
func liveNames() (map[string]bool, error) {
	out, e := exec.Command("tmux", "list-sessions", "-F", "#S").CombinedOutput()
	if e != nil {
		if strings.Contains(string(out), "no server running") || strings.Contains(string(out), "no sessions") || strings.Contains(string(out), "No such file") {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("tmux: %w: %s", e, out)
	}
	m := map[string]bool{}
	for _, s := range strings.Fields(string(out)) {
		if strings.HasPrefix(s, "d-") {
			m[strings.TrimPrefix(s, "d-")] = true
		}
	}
	return m, nil
}
func launch(dir, name string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	cmd := "env " + Quote("PATH="+os.Getenv("PATH")) + " " + Quote(exe) + " dispatch supervise " + Quote(name)
	_, e = command("", "tmux", "new-session", "-d", "-s", "d-"+name, "-c", filepath.Join(dir, "wt"), cmd)
	return e
}
func lockRoot() (func(), error) {
	f, e := os.OpenFile(filepath.Join(Root(), ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func WindowsLaunch(exe, name string) []string { return []string{exe, "dispatch", "supervise", name} }

func startChild(c *exec.Cmd) (func(), error) {
	if e := c.Start(); e != nil {
		return nil, e
	}
	return func() { killTree(c) }, nil
}
