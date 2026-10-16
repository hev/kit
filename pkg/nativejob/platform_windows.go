//go:build windows

package nativejob

import "errors"

// Windows mode bits do not establish a private DACL, and directory FlushFileBuffers
// semantics must be proved on the actual volume. Never silently weaken the Linux
// durability/ownership contract just to make a cross-compiled binary start.
func journalPlatformPreflight() error {
	return errors.New("Windows native journal not enabled: private DACL, directory durability and native restart/ACK acceptance required")
}
