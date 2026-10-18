//go:build linux

package nativehost

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// RunConfig serializes host ownership before removing a stale private socket.
// An old dispatch intent is never removed or replayed during restart.
func RunConfig(ctx context.Context, path string) error {
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
	if s, e := os.Lstat(cfg.Socket); e == nil {
		if s.Mode()&os.ModeSocket == 0 || !owner(s) {
			return ErrHeld
		}
		if os.Remove(cfg.Socket) != nil {
			return ErrHeld
		}
	} else if !os.IsNotExist(e) {
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
