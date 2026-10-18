//go:build !linux && !darwin

package nativegate

import "os"

// Retained journal execution remains fail-closed on unaccepted platforms.
func ownedByCurrentUser(os.FileInfo) bool { return false }
