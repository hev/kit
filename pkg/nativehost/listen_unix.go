//go:build linux || darwin

package nativehost

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// RunConfig serializes host ownership before removing a stale private socket.
// An old dispatch intent is never removed or replayed during restart.
func RunConfig(ctx context.Context, path string) error {
	// An implemented transport is not managed lifecycle acceptance. Keep the
	// role guard before any lock/socket mutation on an unproved platform.
	if !supported() {
		return ErrHeld
	}
	cfg, e := VerifyConfig(path)
	if e != nil {
		return e
	}
	lockPath := filepath.Join(filepath.Dir(cfg.Socket), "host.lock")
	fd, e := syscall.Open(lockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return ErrHeld
	}
	f := os.NewFile(uintptr(fd), lockPath)
	defer f.Close()
	if private(lockPath, false) != nil || syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return ErrHeld
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	if removeStaleSocket(ctx, cfg.Socket, (&net.Dialer{Timeout: time.Second}).DialContext) != nil {
		return ErrHeld
	}
	l, e := net.ListenUnix("unix", &net.UnixAddr{Name: cfg.Socket, Net: "unix"})
	if e != nil {
		return e
	}
	defer l.Close()
	if os.Chmod(cfg.Socket, 0600) != nil {
		return ErrHeld
	}
	return Run(ctx, cfg.Profiles, l)
}

// Only a refused connection proves a stale endpoint. A successful connection or
// any unknown error preserves it. Recheck the inode immediately before unlink.
func removeStaleSocket(ctx context.Context, path string, dial func(context.Context, string, string) (net.Conn, error)) error {
	before, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || before.Mode()&os.ModeSocket == 0 || !owner(before) {
		return ErrHeld
	}
	conn, err := dial(ctx, "unix", path)
	if conn != nil {
		conn.Close()
		return ErrHeld
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return ErrHeld
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !owner(after) {
		return ErrHeld
	}
	if os.Remove(path) != nil {
		return ErrHeld
	}
	return nil
}
