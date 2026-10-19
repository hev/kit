//go:build !windows

package nativejob

import (
	"os"
	"path/filepath"
)

func journalOpenReadOnly(root string) (func() error, error) {
	st, e := os.Lstat(root)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return nil, ErrConflict
	}
	st, e = os.Lstat(filepath.Join(root, "journal.sqlite"))
	if e != nil || !st.Mode().IsRegular() {
		return nil, ErrConflict
	}
	return func() error { return nil }, nil
}
