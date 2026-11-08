//go:build linux || darwin || freebsd || openbsd || netbsd

package maintenance

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

func lockStore(ctx context.Context, path string) (func(), error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	for {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if e != syscall.EWOULDBLOCK && e != syscall.EAGAIN {
			f.Close()
			return nil, e
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Open without blocking on a swapped FIFO and without following a final symlink.
func openRegularSnapshot(path string) (*os.File, error) {
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	fi, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	if !fi.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("maintenance snapshot must be regular file")
	}
	return f, nil
}
