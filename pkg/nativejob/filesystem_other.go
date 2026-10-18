//go:build !windows

package nativejob

import (
	"errors"
	"os"
)

func journalPrepareRoot(root string) error {
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	st, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("journal root must be a private directory (0700)")
	}
	return nil
}

func journalVerifyFile(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("journal file must be regular without a symlink")
	}
	// SQLite files retain the existing Unix permission contract: the private
	// root is the traversal boundary. Do not chmod an existing journal here.
	return nil
}
func journalCreateOutputDir(path string) error { return os.Mkdir(path, 0700) }
func journalCreateOutputFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
}
func journalSyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
