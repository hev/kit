package intake

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

func lockState(path string) (func(), error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	var overlapped syscall.Overlapped
	r, _, e := syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx").Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if r == 0 {
		f.Close()
		return nil, fmt.Errorf("intake already running: %w", e)
	}
	return func() { f.Close() }, nil
}
func replaceState(from, to string) error {
	var e error
	for attempt := 0; attempt < 6; attempt++ {
		e = os.Rename(from, to)
		if e == nil {
			return nil
		}
		if !isSharingViolation(e) {
			return e
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return e
}
func isSharingViolation(e error) bool {
	if p, ok := e.(*os.LinkError); ok {
		return p.Err == syscall.Errno(32) || p.Err == syscall.Errno(33) || p.Err == syscall.Errno(5)
	}
	return false
}
