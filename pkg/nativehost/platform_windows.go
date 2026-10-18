//go:build windows

package nativehost

import (
	"net"
	"os"
)

// These are explicit unsupported Windows adapters, not POSIX permission checks.
func owner(os.FileInfo) bool { return false }
func peer(net.Conn) error    { return ErrHeld }
func supported() bool        { return false }

func startReceiver(Profile, string) (func() error, error) { return nil, ErrHeld }
