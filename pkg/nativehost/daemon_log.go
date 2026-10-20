package nativehost

import "os"

// OpenDaemonLog creates one new process-owned diagnostics file in an existing
// checked private directory. It never truncates, repairs or rotates an old log.
func OpenDaemonLog(root string) (*os.File, error) {
	if !canonical(root) || private(root, true) != nil {
		return nil, ErrHeld
	}
	return platformDaemonLog(root)
}
