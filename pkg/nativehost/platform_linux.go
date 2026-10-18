//go:build linux

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
func peer(c net.Conn) error {
	u, ok := c.(*net.UnixConn)
	if !ok {
		return ErrHeld
	}
	raw, e := u.SyscallConn()
	if e != nil {
		return ErrHeld
	}
	var cred *syscall.Ucred
	var se error
	e = raw.Control(func(fd uintptr) { cred, se = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED) })
	if e != nil || se != nil || cred == nil || cred.Uid != uint32(os.Geteuid()) {
		return ErrHeld
	}
	return nil
}
func supported() bool { return true }
