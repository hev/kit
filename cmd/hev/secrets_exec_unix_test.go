//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestSecretsExecPreservesSignals(t *testing.T) {
	if os.Getenv("HEV_TEST_EXEC_CHILD") == "1" {
		child := exec.Command("sh", "-c", "kill -TERM $$")
		child.Env = os.Environ()
		if err := runSecretsChild(child); err != nil {
			os.Exit(99)
		}
		os.Exit(98)
	}
	c := exec.Command(os.Args[0], "-test.run=^TestSecretsExecPreservesSignals$")
	c.Env = append(os.Environ(), "HEV_TEST_EXEC_CHILD=1")
	err := c.Run()
	e, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected signal death: %v", err)
	}
	status, ok := e.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("lost signal: %v", err)
	}
}

func TestSecretsExecWrapperTermination(t *testing.T) {
	if os.Getenv("HEV_TEST_EXEC_SLEEP") == "1" {
		child := exec.Command("sleep", "60")
		child.Env = os.Environ()
		if err := runSecretsChild(child); err != nil {
			os.Exit(99)
		}
	}
	c := exec.Command(os.Args[0], "-test.run=^TestSecretsExecWrapperTermination$")
	c.Env = append(os.Environ(), "HEV_TEST_EXEC_SLEEP=1")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Process.Kill()
	// Exec retains this PID. Termination cannot orphan a subprocess.
	time.Sleep(200 * time.Millisecond)
	if err := c.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := c.Wait(); err == nil {
		t.Fatal("expected termination")
	}
}
