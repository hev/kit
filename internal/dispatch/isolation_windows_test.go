package dispatch

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func isolateTestProcesses() func() {
	// Admission records refer to the test process. Never query a fixture's PID
	// against the operator's process table, or launch detached/scheduled workers.
	self := os.Getpid()
	created = func(pid int) (uint64, error) {
		if pid == self {
			return 1, nil
		}
		return 0, fmt.Errorf("process %d is not owned by this test", pid)
	}
	launch = func(_, _ string) error { return nil }
	return func() {}
}

// startTestChild grants real probes only for the child this helper starts. Keep
// its creation time as well as its PID so PID reuse cannot admit a foreign process.
// Descendants are contained by startChild's Job and terminated with the root.
func startTestChild(t *testing.T, c *exec.Cmd) {
	t.Helper()
	kill, err := startChild(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(kill)
	pid := c.Process.Pid
	stamp, err := processCreated(pid)
	if err != nil {
		t.Fatal(err)
	}
	previous := created
	created = func(candidate int) (uint64, error) {
		if candidate != pid {
			return previous(candidate)
		}
		current, err := processCreated(candidate)
		if err != nil {
			return 0, err
		}
		if current != stamp {
			return 0, fmt.Errorf("process %d is no longer owned by this test", candidate)
		}
		return current, nil
	}
	t.Cleanup(func() { created = previous })
}

func TestWindowsProcessIsolationGuard(t *testing.T) {
	if _, err := created(os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if _, err := created(os.Getppid()); err == nil {
		t.Fatal("foreign process probe accepted")
	}
	// Invalid launch inputs succeed only through the fake, without a process or task.
	if err := launch("missing", "invalid/name"); err != nil {
		t.Fatal(err)
	}
}
