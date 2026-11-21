//go:build !windows

package hooks

import (
	"context"
	"os/exec"
	"syscall"
)

// runHandler runs a user handler in its own process group, and cancellation
// kills the whole group: a handler's children must not outlive the hook.
func runHandler(_ context.Context, c *exec.Cmd) error {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	return c.Run()
}

func handlerCommand(ctx context.Context, exe, event string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, exe, event), nil
}
