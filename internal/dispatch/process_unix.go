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

type pidRecord struct {
	PID int `json:"pid"`
}

func writeProcessRecord(path string, pid int) error { return WriteJSON(path, pidRecord{pid}) }
func processRecordLive(path string) bool {
	var p pidRecord
	return readJSON(path, &p) == nil && p.PID > 0 && syscall.Kill(p.PID, 0) == nil
}
func writePID(dir string) error {
	return writeProcessRecord(filepath.Join(dir, "supervisor.json"), os.Getpid())
}
func terminationSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}
}
func recordHarness(dir string, c *exec.Cmd) error {
	return writeProcessRecord(filepath.Join(dir, "harness.json"), c.Process.Pid)
}
func stopHarness(dir string) error {
	path := filepath.Join(dir, "harness.json")
	var p pidRecord
	if readJSON(path, &p) != nil || p.PID <= 0 {
		return nil
	}
	err := syscall.Kill(-p.PID, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return nil
	}
	return err
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
	cmd := "exec " + Quote(exe) + " dispatch supervise -- " + Quote(name)
	args := []string{"new-session", "-d", "-s", "d-" + name, "-c", filepath.Join(dir, "wt")}
	for _, value := range launchEnvironment(dir) {
		args = append(args, "-e", value)
	}
	args = append(args, cmd)
	c := exec.Command("tmux", args...)
	if e = c.Run(); e != nil {
		return fmt.Errorf("tmux launch: %w", e)
	}
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
func WindowsLaunch(exe, name string) []string {
	return []string{exe, "dispatch", "supervise", "--", name}
}

func startChild(c *exec.Cmd) (func(), error) {
	if e := c.Start(); e != nil {
		return nil, e
	}
	return func() { killTree(c) }, nil
}
