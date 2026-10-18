//go:build windows

package nativehost

import (
	"net"
	"os"

	"github.com/hev/kit/internal/winpipe"
	"github.com/hev/kit/internal/winplatform"
)

// FileInfo mode/UID is not Windows owner proof. Path-based checked-handle
// filesystem hooks are integrated separately; this must never infer privacy.
func owner(os.FileInfo) bool { return false }
func peer(conn net.Conn) error {
	c, ok := conn.(*winpipe.Conn)
	if !ok {
		return ErrHeld
	}
	return c.Authenticate()
}

// This is the controlled candidate capability, not managed readiness. Session
// zero remains held, and opening still verifies private pipe/config ownership.
func supported() bool {
	_, err := windowsUserSession()
	return winplatform.ExecutionBuild() && err == nil
}
