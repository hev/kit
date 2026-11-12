package dispatch

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Infer identity from the recorded harness process, even if a descendant clears
// its environment. This is defense in depth, not an OS sandbox.
func roundProcessIdentity() string {
	parents, e := processParents()
	if e != nil {
		return ""
	}
	seen := map[int]bool{}
	for pid := os.Getpid(); pid > 0 && !seen[pid]; {
		seen[pid] = true
		if name := roundIdentityForAncestors(map[int]bool{pid: true}); name != "" {
			return name
		}
		parent, ok := parents[pid]
		// Windows keeps a parent PID after the parent exits, and the PID may
		// be reused. A real parent was created before its child.
		if !ok || !createdBefore(parent, pid) {
			break
		}
		pid = parent
	}
	return ""
}

// processParents snapshots the process table once, through Toolhelp.
func processParents() (map[int]int, error) {
	snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if e != nil {
		return nil, e
	}
	defer windows.CloseHandle(snapshot)
	parents := map[int]int{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for e = windows.Process32First(snapshot, &entry); e == nil; e = windows.Process32Next(snapshot, &entry) {
		parents[int(entry.ProcessID)] = int(entry.ParentProcessID)
	}
	if e != windows.ERROR_NO_MORE_FILES {
		return nil, e
	}
	return parents, nil
}

func createdBefore(parent, child int) bool {
	p, e := processCreated(parent)
	if e != nil {
		return false
	}
	c, e := processCreated(child)
	return e == nil && p <= c
}
