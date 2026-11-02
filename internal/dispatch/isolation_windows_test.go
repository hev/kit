package dispatch

import (
	"fmt"
	"os"
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

func TestWindowsProcessIsolationGuard(t *testing.T) {
	if _, err := created(os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if _, err := created(0); err == nil {
		t.Fatal("foreign process probe accepted")
	}
	// Invalid launch inputs succeed only through the fake, without a process or task.
	if err := launch("missing", "invalid/name"); err != nil {
		t.Fatal(err)
	}
}
