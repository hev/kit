//go:build windows

package nativehost

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"time"

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
func openEndpoint(ctx context.Context, configPath string, cfg Config) (net.Listener, error) {
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
