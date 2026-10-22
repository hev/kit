//go:build !windows

// Package platformops implements native writer locking, private state files,
// and daemon lifecycle operations.
package platformops

import (
	"context"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
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

func securePath(path string, mode os.FileMode) error                     { return os.Chmod(path, mode) }
func SyncDir(f *os.File) error                                           { return f.Sync() }
func Rename(old, new string) error                                       { return os.Rename(old, new) }
func DaemonContext(ctx context.Context) (context.Context, func(), error) { return ctx, func() {}, nil }
