//go:build windows

package windowsfs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// OpenExisting validates the actual handle before exposing an existing regular
// owner-private file. It neither creates, repairs nor truncates the object.
func OpenExisting(path string, writable bool) (*os.File, error) {
	access := uint32(windows.GENERIC_READ)
	if writable {
		access |= windows.GENERIC_WRITE
	}
	h, err := JournalHandle(path, access, false)
	if err != nil {
		return nil, err
	}
	if err = VerifyPrivateHandle(h, false, false); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(h))
	}
	return os.NewFile(uintptr(h), path), nil
}

// Lock holds an exclusive byte-range lock on an owner-private lock file. The
// handle is never inherited. There is no PID-only adoption or stale-file unlink.
type Lock struct {
	mu   sync.Mutex
	file *os.File
	path string
	held bool
}

func Acquire(ctx context.Context, path string) (*Lock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := CreatePrivateFile(path)
	created := err == nil
	if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		f, err = OpenExisting(path, true)
	}
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Lock, error) { return nil, errors.Join(err, f.Close()) }
	if created {
		if err = errors.Join(f.Sync(), SyncDirectory(filepath.Dir(path))); err != nil {
			return fail(err)
		}
	}
	var ov windows.Overlapped
	for {
		if err = ctx.Err(); err != nil {
			return fail(err)
		}
		err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ov)
		if err == nil {
			return &Lock{file: f, path: path, held: true}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return fail(err)
		}
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (l *Lock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		return nil
	}
	l.held = false
	var ov windows.Overlapped
	return errors.Join(windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &ov), l.file.Close())
}

// Publish holds this exact ownership exclusion through a final ownership/expiry
// callback and replacement. The callback must not call this Lock recursively.
// Errors after replacement can mean committed state: reconcile before retry.
// Target and lock must share their owner-private directory; no cross-volume copy.
func (l *Lock) Publish(ctx context.Context, target string, data []byte, finalCheck func() error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held || finalCheck == nil || filepath.Clean(filepath.Dir(target)) != filepath.Clean(filepath.Dir(l.path)) {
		return errors.New("held ownership lock, same-directory target and final check required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(target), ".native-publish-"+hex.EncodeToString(random[:]))
	f, err := CreatePrivateFile(temp)
	if err != nil {
		return err
	}
	// Only this operation's exclusive temporary name is eligible for removal.
	defer os.Remove(temp)
	n, writeErr := f.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if err = errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return replacePrivateFile(temp, target, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := finalCheck(); err != nil {
			return err
		}
		return ctx.Err()
	})
}
