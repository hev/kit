//go:build windows

package dispatch

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/hev/kit/internal/platformops"
)

// lockHeld reports whether another handle holds dir's lockDirectory lock. A
// probe failure other than a busy lock or a missing directory panics, so it
// cannot pass as an unheld lock.
func lockHeld(dir string) bool {
	unlock, err := platformops.LockPath(filepath.Join(dir, ".lock"), true)
	switch {
	case err == nil:
		unlock()
		return false
	case platformops.IsLockBusy(err):
		return true
	case errors.Is(err, fs.ErrNotExist):
		return false
	}
	panic("probe root lock: " + err.Error())
}
