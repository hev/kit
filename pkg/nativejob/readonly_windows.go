//go:build windows

package nativejob

import (
	"errors"
	"path/filepath"
	"sync"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	"golang.org/x/sys/windows"
)

// journalOpenReadOnly holds the existing original private artifacts while
// SQLite performs its RO snapshot. Missing sidecars hold; this observer never
// initializes a journal, repairs ACLs or creates WAL/SHM on behalf of SQLite.
func journalOpenReadOnly(root string) (close func() error, err error) {
	if fs.PathSyntax(root) != nil {
		return nil, ErrConflict
	}
	dir, err := fs.JournalHandle(root, 0, true)
	if err != nil {
		return nil, err
	}
	if err = fs.VerifyPrivateHandle(dir, true, true); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(dir))
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(dir, &info); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(dir))
	}
	expected := fs.FileIdentity{Device: uint64(info.VolumeSerialNumber), Inode: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}
	leases := []*fs.FileLease{}
	var once sync.Once
	var result error
	cleanup := func() error {
		once.Do(func() {
			actual, e := fs.PrivateIdentity(root, true)
			result = errors.Join(result, e)
			if actual != expected {
				result = errors.Join(result, ErrConflict)
			}
			for i := len(leases) - 1; i >= 0; i-- {
				result = errors.Join(result, leases[i].Recheck(), leases[i].Close())
			}
			result = errors.Join(result, windows.CloseHandle(dir))
		})
		return result
	}

	for _, name := range []string{"journal.sqlite", "journal.sqlite-wal", "journal.sqlite-shm"} {
		lease, e := fs.OpenFileLease(filepath.Join(root, name), true)
		if e != nil {
			return nil, errors.Join(e, cleanup())
		}
		leases = append(leases, lease)
	}
	return cleanup, nil
}
