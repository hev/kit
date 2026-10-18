//go:build windows

package nativehost

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/hev/kit/internal/winpipe"
	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
)

func validateEndpoint(path string) error { return winpipe.Validate(path) }

type windowsOwnerListener struct {
	*winpipe.Listener
	lock *fs.Lock
	once sync.Once
	err  error
}

func (l *windowsOwnerListener) Close() error {
	l.once.Do(func() { l.err = errors.Join(l.Listener.Close(), l.lock.Close()) })
	return l.err
}

// windowsUserSession checks the server process token, not the controller's
// session. Session zero cannot host the supported user coding runtime. A
// nonzero session is necessary, not proof of runtime or lifecycle readiness.
func windowsUserSession() (uint32, error) {
	var session, size uint32
	err := windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &size)
	if err != nil || size != 4 || session == 0 {
		return 0, ErrHeld
	}
	return session, nil
}
func openEndpoint(ctx context.Context, configPath string, cfg Config) (net.Listener, error) {
	if _, err := windowsUserSession(); err != nil {
		return nil, err
	}
	if ctx.Err() != nil || fs.VerifyPrivate(configPath, true, false) != nil || fs.VerifyPrivate(filepath.Dir(configPath), true, true) != nil {
		return nil, ErrHeld
	}
	expected, err := winpipe.Name(configPath)
	if err != nil || cfg.Socket != expected {
		return nil, ErrHeld
	}
	// The same checked private config directory owns the durable endpoint lock.
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock, err := fs.Acquire(lockCtx, filepath.Join(filepath.Dir(configPath), "host.lock"))
	if err != nil {
		return nil, err
	}
	pipe, err := winpipe.Listen(cfg.Socket)
	if err != nil {
		return nil, errors.Join(err, lock.Close())
	}
	return &windowsOwnerListener{Listener: pipe, lock: lock}, nil
}
func verifyListener(listener net.Listener) error {
	l, ok := listener.(*windowsOwnerListener)
	if !ok || !l.Listener.Attested() {
		return ErrHeld
	}
	return nil
}
func dialEndpoint(ctx context.Context, path string) (net.Conn, error) { return winpipe.Dial(ctx, path) }
func readRequest(conn net.Conn) ([]byte, error)                       { return winpipe.ReadRequest(conn) }
func writeRequest(conn net.Conn, payload []byte) error                { return winpipe.WriteRequest(conn, payload) }
