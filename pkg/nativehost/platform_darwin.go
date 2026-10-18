//go:build darwin

package nativehost

import (
	"net"
	"os"
	"syscall"
)

func owner(s os.FileInfo) bool {
	v, ok := s.Sys().(*syscall.Stat_t)
	return ok && v.Uid == uint32(os.Geteuid())
}
func peer(net.Conn) error { return ErrHeld }
func supported() bool     { return false }
