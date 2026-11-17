//go:build windows

package dispatch

import (
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// interruptHarness has no portable console interrupt for a detached harness
// on Windows; the halt waits for its hooks or the hard-stop timeout.
func interruptHarness(dir string) error {
	return fmt.Errorf("no harness interrupt on Windows; the halt waits for kit hooks or the hard stop")
}

// startDetached starts c outside the supervisor's console and process group.
func startDetached(c *exec.Cmd) error {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
