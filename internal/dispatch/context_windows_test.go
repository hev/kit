package dispatch

import (
	"fmt"
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestWindowsContextLifecycle runs the recipe lifecycle on the Windows launch
// path: job-object harness start with the bundle on standard input.
func TestWindowsContextLifecycle(t *testing.T) {
	allowHarnessProbes(t)
	runContextLifecycle(t)
}

// allowHarnessProbes lets a lifecycle test's supervisor record the harness
// children it starts. The suite refuses process probes for anything but the
// test itself (see isolateTestProcesses); this grants real probes for direct
// children of this test process only, never for the operator's other
// processes.
func allowHarnessProbes(t *testing.T) {
	previous := created
	self := uint32(os.Getpid())
	created = func(pid int) (uint64, error) {
		if parent, err := parentProcess(uint32(pid)); err == nil && parent == self {
			return processCreated(pid)
		}
		return previous(pid)
	}
	t.Cleanup(func() { created = previous })
}

func parentProcess(pid uint32) (uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == pid {
			return entry.ParentProcessID, nil
		}
	}
	return 0, fmt.Errorf("process %d not found", pid)
}
