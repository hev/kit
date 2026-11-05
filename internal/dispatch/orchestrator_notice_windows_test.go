//go:build windows

package dispatch

import (
	"path/filepath"
	"testing"
)

// lockHeld reports whether dir's lockDirectory lock directory exists.
func lockHeld(dir string) bool { return exists(filepath.Join(dir, ".lockdir")) }

// The Windows CI selector runs Windows-prefixed tests only.
func TestWindowsNoticeSendHoldsNoLock(t *testing.T) { TestNoticeSendHoldsNoLock(t) }
