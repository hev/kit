package feed

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// startDetached starts c outside the hook's console and process group.
func startDetached(c *exec.Cmd) error {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
