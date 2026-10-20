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

// Build capability is shared with clients, which may run in Session0. Server
// openEndpoint independently refuses Session0 before lock or pipe mutation.
// Neither check establishes managed readiness or launch authority.
func supported() bool { return winplatform.ExecutionBuild() }
