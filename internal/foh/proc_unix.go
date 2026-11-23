//go:build !windows

package foh

import (
	"os/exec"
	"syscall"
)

// setProcessGroup starts cmd in its own process group so a timeout kills the
// whole tree this tool call started, and nothing else.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
