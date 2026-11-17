//go:build !windows

package hooks

import (
	"os/exec"
	"syscall"
)

// ownGroup runs a user handler in its own process group, and cancellation
// kills the whole group: a handler's children must not outlive the hook.
func ownGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
