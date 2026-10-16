//go:build !windows

// Package platformops preserves Unix writer locking and lifecycle operations.
// Windows support is deliberately fail-closed until native acceptance exists.
package platformops

import (
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func RequireTraceWrites() error { return nil }
func LockFile(f *os.File, nonblocking bool) error {
	flags := unix.LOCK_EX
	if nonblocking {
		flags |= unix.LOCK_NB
	}
	return unix.Flock(int(f.Fd()), flags)
}
func UnlockFile(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func LockPath(path string, nonblocking bool) (func(), error) {
	fd, e := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	flags := unix.LOCK_EX
	if nonblocking {
		flags |= unix.LOCK_NB
	}
	if e = unix.Flock(fd, flags); e != nil {
		unix.Close(fd)
		return nil, e
	}
	return func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }, nil
}
func IsLockBusy(err error) bool { return err == syscall.EWOULDBLOCK || err == syscall.EAGAIN }
func SignalPID(pid int, force bool) error {
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	return syscall.Kill(pid, signal)
}
func ProcessGone(err error) bool { return err == syscall.ESRCH }
