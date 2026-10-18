//go:build windows

package nativehost

import (
	"net"
	"os"

	"github.com/hev/kit/internal/winpipe"
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

// Source hooks and isolated tests are not managed lifecycle acceptance.
func supported() bool { return false }
