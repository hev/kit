//go:build !linux && !darwin

package nativehost

import (
	"net"
	"os"
)

func owner(os.FileInfo) bool { return false }
func peer(net.Conn) error    { return ErrHeld }
func supported() bool        { return false }
