//go:build windows

package dispatch

import "path/filepath"

// lockHeld reports whether dir's lockDirectory lock directory exists.
func lockHeld(dir string) bool { return exists(filepath.Join(dir, ".lockdir")) }
