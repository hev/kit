//go:build !windows

package businessflow

import (
	"os"
	"syscall"
)

func mutationSupported() error    { return nil }
func lockFile(f *os.File) error   { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }
func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
func openSnapshot(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
}
func openExistingLock(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|syscall.O_NOFOLLOW, 0)
}
