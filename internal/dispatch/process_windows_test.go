package dispatch

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestWindowsContainmentFlags(t *testing.T) {
	flags := windowsSupervisorFlags()
	for _, flag := range []uint32{windows.CREATE_BREAKAWAY_FROM_JOB, windows.DETACHED_PROCESS, windows.CREATE_NEW_PROCESS_GROUP} {
		if flags&flag == 0 {
			t.Fatalf("supervisor missing creation flag %#x", flag)
		}
	}
	if windowsHarnessFlags()&windows.CREATE_SUSPENDED == 0 {
		t.Fatal("harness must not execute before job assignment")
	}
}
