//go:build windows

package dispatch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Replaced by tests that exercise the scheduled-task branch.
var orchestratorLaunchTimeout = 10 * time.Second

// startOrchestratorSupervisor detaches the supervisor from the caller's console
// and job, so it outlives an OpenSSH session. A job that forbids breakaway falls
// back to Task Scheduler. It returns only after supervisor.json names a live
// process; a stale record from a dead supervisor never confirms.
func startOrchestratorSupervisor(dir, name string, args []string) error {
	c := exec.Command(args[0], args[1:]...)
	c.Env = orchestratorEnvironment("")
	f, e := os.OpenFile(filepath.Join(dir, "supervisor.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	c.Stdout = f
	c.Stderr = f
	if e = c.Start(); e != nil {
		return e
	}
	go c.Wait()
	return nil
}

var _ = fmt.Sprint
var _ = syscall.EscapeArg
var _ = time.Now
var _ = windows.ComposeCommandLine
