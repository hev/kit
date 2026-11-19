//go:build !windows

package feed

import (
	"os/exec"
	"syscall"
)

// startDetached starts c in its own session, so a hook process group being
// killed does not take the sender with it.
func startDetached(c *exec.Cmd) error {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
