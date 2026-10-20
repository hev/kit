//go:build darwin

package nativehost

import (
	n "github.com/hev/kit/pkg/nativejob"
	"golang.org/x/sys/unix"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// This private ABI is deliberately limited to the exact native-tested kernel
// build and architecture. Constants336/459 are also present in that build's
// installed SDK sys/syscall.h. Unsupported builds must not silently inherit it.
func observeResourceCoalition(expected n.Birth, expectedCID uint64) (CoalitionObservation, error) {
	build, e := unix.Sysctl("kern.osversion")
	if e != nil || runtime.GOARCH != "arm64" || build != "25G83" {
		return CoalitionObservation{}, ErrHeld
	}
	before, e := n.ProcessBirth(expected.PID)
	if e != nil || before != expected {
		return CoalitionObservation{}, ErrHeld
	}
	cid, e := darwinResourceCID(expected.PID)
	if e != nil || cid == 0 || (expectedCID != 0 && expectedCID != cid) {
		return CoalitionObservation{}, ErrHeld
	}
	var counts [2]uint64
	size := uint64(16)
	rc, _, errno := syscall.Syscall6(459, 1, uintptr(unsafe.Pointer(&cid)), uintptr(unsafe.Pointer(&counts[0])), uintptr(unsafe.Pointer(&size)), 0, 0)
	runtime.KeepAlive(&cid)
	runtime.KeepAlive(&counts)
	runtime.KeepAlive(&size)
	if errno != 0 || rc != 0 || size != 16 || counts[0] == 0 || counts[0] < counts[1] || counts[0]-counts[1] < 1 {
		return CoalitionObservation{}, ErrHeld
	}
	afterCID, e := darwinResourceCID(expected.PID)
	if e != nil || afterCID != cid {
		return CoalitionObservation{}, ErrHeld
	}
	after, e := n.ProcessBirth(expected.PID)
	if e != nil || after != expected {
		return CoalitionObservation{}, ErrHeld
	}
	return CoalitionObservation{CoalitionVersion, expected, cid, counts[0], counts[1], build, time.Now().UTC()}, nil
}
func darwinResourceCID(pid int) (uint64, error) {
	var ids [5]uint64
	size, _, errno := syscall.Syscall6(336, 2, uintptr(pid), 20, 0, uintptr(unsafe.Pointer(&ids[0])), 40)
	runtime.KeepAlive(&ids)
	if errno != 0 || size != 40 || ids[0] == 0 {
		return 0, ErrHeld
	}
	return ids[0], nil
}
