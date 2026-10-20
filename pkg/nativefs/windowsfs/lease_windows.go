//go:build windows

package windowsfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

// ErrLeaseBusy denotes only a conflicting Windows file sharing reservation.
// Authentication, unsupported filesystem and metadata failures remain unknown.
var ErrLeaseBusy = errors.New("private file lease busy")

type FileIdentity struct{ Device, Inode uint64 }

// FileLease holds an existing permanent private file object, without byte-range
// locks. Shared leases deny write/delete opens; exclusive leases deny all data
// opens. Duplication preserves the SAME object's sharing reservation until its
// last handle closes. Closing a child early is not proof its whole tree is gone.
// The execution owner must independently retain tree/reference authority.
type FileLease struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	shared   bool
	identity FileIdentity
}

// OpenFileLease never creates, repairs or replaces a path. Acquisition is a
// nonblocking try; callers classify only ErrLeaseBusy as known contention.
func OpenFileLease(path string, shared bool) (*FileLease, error) {
	if filepath.Clean(path) != path {
		return nil, errors.New("lease requires canonical path")
	}
	if err := VerifyPrivate(filepath.Dir(path), true, true); err != nil {
		return nil, err
	}
	access, share := uint32(windows.GENERIC_READ|windows.GENERIC_WRITE), uint32(0)
	if shared {
		access = windows.GENERIC_READ
		share = windows.FILE_SHARE_READ
	}
	h, err := checkedHandleSharing(path, access, false, true, share)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, fmt.Errorf("%w: %w", ErrLeaseBusy, err)
		}
		return nil, err
	}
	if err = VerifyPrivateHandle(h, true, false); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(h))
	}
	identity, err := leaseIdentity(h)
	if err != nil {
		return nil, errors.Join(err, windows.CloseHandle(h))
	}
	l := &FileLease{file: os.NewFile(uintptr(h), path), path: path, shared: shared, identity: identity}
	if err = l.Recheck(); err != nil {
		return nil, errors.Join(err, l.Close())
	}
	return l, nil
}
func leaseIdentity(h windows.Handle) (FileIdentity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return FileIdentity{}, err
	}
	return FileIdentity{Device: uint64(info.VolumeSerialNumber), Inode: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}, nil
}
func (l *FileLease) checkLocked() error {
	if l.file == nil {
		return os.ErrClosed
	}
	held := windows.Handle(l.file.Fd())
	if err := VerifyPrivateHandle(held, true, false); err != nil {
		return err
	}
	id, err := leaseIdentity(held)
	if err != nil {
		return err
	}
	if id != l.identity {
		return errors.New("held lease file identity changed")
	}
	// Metadata-only access does not conflict with our exclusive data-sharing
	// reservation. Never reopen for data access or silently release the lease.
	current, err := checkedHandleSharing(l.path, windows.FILE_READ_ATTRIBUTES, false, true, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
	if err != nil {
		return err
	}
	verifyErr := VerifyPrivateHandle(current, true, false)
	actual, idErr := leaseIdentity(current)
	closeErr := windows.CloseHandle(current)
	if err = errors.Join(verifyErr, idErr, closeErr); err != nil {
		return err
	}
	if actual != id {
		return errors.New("permanent lease pathname replaced")
	}
	return nil
}
func (l *FileLease) Recheck() error {
	if l == nil {
		return os.ErrClosed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.checkLocked()
}
func (l *FileLease) Identity() (FileIdentity, error) {
	if l == nil {
		return FileIdentity{}, os.ErrClosed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.checkLocked(); err != nil {
		return FileIdentity{}, err
	}
	return l.identity, nil
}

// DuplicateForChild returns only a read-only, NONINHERITABLE duplicate of the
// already checked shared lease. The fixed platform spawn adapter owns any
// bounded inheritance window, explicit handle allowlist and failure cleanup.
// No process identifier, arbitrary handle or additional access is accepted.
func (l *FileLease) DuplicateForChild() (*os.File, error) {
	if l == nil {
		return nil, os.ErrClosed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.shared {
		return nil, errors.New("exclusive lease cannot be inherited by consumer")
	}
	if err := l.checkLocked(); err != nil {
		return nil, err
	}
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(l.file.Fd()), windows.CurrentProcess(), &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(duplicate), "private-lease-child"), nil
}

// Close releases only this handle, never an inherited child's reservation.
func (l *FileLease) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// PrivateIdentity reads only checked private-object metadata, including directory
// identities used by reference stores. It grants no data or mutation handle.
func PrivateIdentity(path string, directory bool) (FileIdentity, error) {
	h, err := JournalHandle(path, windows.FILE_READ_ATTRIBUTES, directory)
	if err != nil {
		return FileIdentity{}, err
	}
	verifyErr := VerifyPrivateHandle(h, true, directory)
	id, idErr := leaseIdentity(h)
	if err = errors.Join(verifyErr, idErr, windows.CloseHandle(h)); err != nil {
		return FileIdentity{}, err
	}
	return id, nil
}
