//go:build windows

package hygiene

import "os"

// Unix /tmp UID-based pruning is not performed on Windows.
func isOwnedByCurrentUser(os.FileInfo) bool {
	return false
}
