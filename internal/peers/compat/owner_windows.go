//go:build windows

package compat

import "os"

// Windows has no POSIX owner; the private state directory is the boundary.
func ownedByMe(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
