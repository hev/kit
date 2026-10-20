//go:build linux || darwin

package nativehost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// openEndpoint retains the owner lock until the returned listener closes.
func openEndpoint(ctx context.Context, configPath string, cfg Config) (net.Listener, error) {
	if ctx.Err() != nil || private(configPath, false) != nil || validateEndpoint(cfg.Socket) != nil {
		return nil, ErrHeld
	}
	lockPath := filepath.Join(filepath.Dir(cfg.Socket), "host.lock")
	fd, e := syscall.Open(lockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, ErrHeld
	}
	f := os.NewFile(uintptr(fd), lockPath)
	transferred := false
	defer func() {
		if !transferred {
			_ = f.Close()
		}
	}()
	if private(lockPath, false) != nil || syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil, ErrHeld
	}
	defer func() {
		if !transferred {
			_ = syscall.Flock(fd, syscall.LOCK_UN)
		}
	}()
	if removeStaleSocket(ctx, cfg.Socket, (&net.Dialer{Timeout: time.Second}).DialContext) != nil {
		return nil, ErrHeld
	}
	l, e := net.ListenUnix("unix", &net.UnixAddr{Name: cfg.Socket, Net: "unix"})
	if e != nil {
		return nil, e
	}

	if os.Chmod(cfg.Socket, 0600) != nil {
		_ = l.Close()
		return nil, ErrHeld
	}
	transferred = true
	return &unixOwnedListener{UnixListener: l, lock: f}, nil
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

type unixOwnedListener struct {
	*net.UnixListener
	lock     *os.File
	once     sync.Once
	closeErr error
}

func (l *unixOwnedListener) Close() error {
	l.once.Do(func() {
		l.closeErr = l.UnixListener.Close()
		_ = syscall.Flock(int(l.lock.Fd()), syscall.LOCK_UN)
		_ = l.lock.Close()
	})
	return l.closeErr
}
func validateEndpoint(path string) error {
	if !canonical(path) || private(filepath.Dir(path), true) != nil {
		return ErrHeld
	}
	return nil
}
func verifyListener(listener net.Listener) error {
	var u *net.UnixListener
	switch l := listener.(type) {
	case *net.UnixListener:
		u = l
	case *unixOwnedListener:
		u = l.UnixListener
	default:
		return ErrHeld
	}
	if u == nil {
		return ErrHeld
	}
	path := u.Addr().String()
	s, e := os.Lstat(path)
	if e != nil || s.Mode()&os.ModeSocket == 0 || s.Mode().Perm() != 0600 || !owner(s) || validateEndpoint(path) != nil {
		return ErrHeld
	}
	return nil
}
func dialEndpoint(ctx context.Context, path string) (net.Conn, error) {
	if validateEndpoint(path) != nil {
		return nil, ErrHeld
	}
	s, e := os.Lstat(path)
	if e != nil || s.Mode()&os.ModeSocket == 0 || s.Mode().Perm() != 0600 || !owner(s) {
		return nil, ErrHeld
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", path)
}
func readRequest(conn net.Conn) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(conn, maxFrame+1))
	if e != nil || len(b) > maxFrame {
		return nil, ErrHeld
	}
	return b, nil
}
func writeRequest(conn net.Conn, payload []byte) error {
	if len(payload) > maxFrame {
		return ErrHeld
	}
	u, ok := conn.(*net.UnixConn)
	if !ok {
		return ErrHeld
	}
	if _, e := io.Copy(u, bytes.NewReader(payload)); e != nil {
		return ErrHeld
	}
	return u.CloseWrite()
}
