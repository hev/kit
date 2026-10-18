//go:build windows

package nativejob

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/hev/kit/pkg/nativefs/windowsfs"
	"golang.org/x/sys/windows"
)

// The journal and host use the same reviewed selected-account Windows policy.
// These aliases preserve package-local test access; they do not enable Open.
func windowsPrivateDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	return windowsfs.PrivateDescriptor()
}
func windowsPathSyntax(path string) error { return windowsfs.PathSyntax(path) }
func windowsLocalPath(path string) error  { return windowsfs.LocalPath(path) }
func windowsJournalHandle(path string, access uint32, dir bool) (windows.Handle, error) {
	return windowsfs.JournalHandle(path, access, dir)
}
func windowsVerifyPrivateHandle(h windows.Handle, protected, dir bool) error {
	return windowsfs.VerifyPrivateHandle(h, protected, dir)
}
func windowsVerifyPrivate(path string, protected, dir bool) error {
	return windowsfs.VerifyPrivate(path, protected, dir)
}
func windowsCreatePrivateDir(path string, existing bool) error {
	return windowsfs.CreatePrivateDir(path, existing)
}
func windowsCreatePrivateFile(path string) (*os.File, error) {
	return windowsfs.CreatePrivateFile(path)
}
func windowsSyncDirectory(path string) error { return windowsfs.SyncDirectory(path) }
func windowsReplacePrivateFile(temp, target string) error {
	return windowsfs.ReplacePrivateFile(temp, target)
}

func journalPrepareRoot(root string) error                  { return windowsCreatePrivateDir(root, true) }
func journalVerifyFile(path string) error                   { return windowsVerifyPrivate(path, false, false) }
func journalCreateOutputDir(path string) error              { return windowsCreatePrivateDir(path, false) }
func journalCreateOutputFile(path string) (*os.File, error) { return windowsCreatePrivateFile(path) }
func journalSyncDir(path string) error                      { return windowsSyncDirectory(path) }

// SQLite inherits the root DACL but Windows can choose a token's default owner
// group rather than its user. Create all persistent files with the explicit
// descriptor before SQLite opens them; never repair an existing foreign owner.
func journalPrepareDatabase(root string) error {
	for _, name := range []string{"journal.sqlite", "journal.sqlite-wal", "journal.sqlite-shm"} {
		path := filepath.Join(root, name)
		if err := journalVerifyFile(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		f, err := windowsCreatePrivateFile(path)
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			if err = journalVerifyFile(path); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if err = errors.Join(f.Sync(), f.Close()); err != nil {
			return err
		}
	}
	return nil
}
