//go:build linux || darwin

package nativehost

import (
	"golang.org/x/sys/unix"
	"os"
)

func checkedOpen(path string, private, executable bool) (*os.File, error) {
	if !canonical(path) {
		return nil, ErrHeld
	}
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, ErrHeld
	}
	f := os.NewFile(uintptr(fd), path)
	s, e := f.Stat()
	if e != nil || !s.Mode().IsRegular() || s.Mode().Perm()&0022 != 0 || (private && (!owner(s) || s.Mode().Perm()&0077 != 0)) || (executable && s.Mode().Perm()&0111 == 0) {
		f.Close()
		return nil, ErrHeld
	}
	return f, nil
}
func openPrivate(path string) (*os.File, error) { return checkedOpen(path, true, false) }
func openPinned(path string, executable bool) (*os.File, error) {
	return checkedOpen(path, false, executable)
}
func platformPrivate(path string, dir bool) error {
	if !dir {
		f, e := openPrivate(path)
		if e != nil {
			return e
		}
		return f.Close()
	}
	if !canonical(path) {
		return ErrHeld
	}
	s, e := os.Lstat(path)
	if e != nil || !s.IsDir() || !owner(s) || s.Mode().Perm()&0077 != 0 {
		return ErrHeld
	}
	return nil
}
func platformPinned(path string, executable bool) error {
	f, e := openPinned(path, executable)
	if e != nil {
		return e
	}
	return f.Close()
}
