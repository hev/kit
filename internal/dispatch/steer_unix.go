//go:build !windows

package dispatch

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
)

// interruptHarness sends a headless harness its own interrupt (SIGINT), the
// fallback halt for a launch without kit hooks. It never kills.
func interruptHarness(dir string) error {
	path := filepath.Join(dir, "harness.json")
	var p pidRecord
	if !processRecordLive(path) || readJSON(path, &p) != nil {
		return fmt.Errorf("no live harness process recorded")
	}
	return syscall.Kill(p.PID, syscall.SIGINT)
}

// startDetached starts c in its own session so it outlives the supervisor's
// tmux session.
func startDetached(c *exec.Cmd) error {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
