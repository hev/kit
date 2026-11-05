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

// orchestratorLaunchTimeout bounds start confirmation, including a supervisor
// that waits briefly for the root lock.
const orchestratorLaunchTimeout = 10 * time.Second

// startOrchestratorSupervisor detaches the supervisor from the caller's console
// and job, so it outlives an OpenSSH session. A job that forbids breakaway falls
// back to Task Scheduler. It returns only after supervisor.json names a live
// process; a stale record from a dead supervisor never confirms.
func startOrchestratorSupervisor(dir, name string, args []string) error {
	log := filepath.Join(dir, "supervisor.log")
	f, e := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	c := exec.Command(args[0], args[1:]...)
	c.Env = orchestratorEnvironment("") // Code gathering uses the child spec; sessions get its identity.
	c.Stdout = f
	c.Stderr = f
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsSupervisorFlags()}
	var exited chan error
	if e = c.Start(); e != nil {
		// Task Scheduler starts outside the SSH service's restrictive job.
		task := "hev-orchestrator-" + name
		if _, err := command("", "schtasks", "/Create", "/TN", task, "/TR", windows.ComposeCommandLine(args), "/SC", "ONCE", "/ST", "00:00", "/F"); err != nil {
			return fmt.Errorf("breakaway: %v; scheduled task: %w", e, err)
		}
		defer command("", "schtasks", "/Delete", "/TN", task, "/F")
		if _, err := command("", "schtasks", "/Run", "/TN", task); err != nil {
			return err
		}
	} else {
		exited = make(chan error, 1)
		go func() { exited <- c.Wait() }()
	}
	record := filepath.Join(dir, "supervisor.json")
	deadline := time.Now().Add(orchestratorLaunchTimeout)
	for {
		if processRecordLive(record) {
			return nil
		}
		select {
		case err := <-exited:
			// A concurrent launch may own the live record.
			if processRecordLive(record) {
				return nil
			}
			return fmt.Errorf("orchestrator supervisor exited before confirming (%v); see %s", err, log)
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w; see %s", errLaunchUnconfirmed, log)
		}
	}
}
