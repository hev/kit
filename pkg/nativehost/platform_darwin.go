//go:build darwin

package nativehost

import (
	n "github.com/hev/kit/pkg/nativejob"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"syscall"
)

func owner(s os.FileInfo) bool {
	v, ok := s.Sys().(*syscall.Stat_t)
	return ok && v.Uid == uint32(os.Geteuid())
}

// peer authenticates both sides of the existing Unix transport with kernel
// credentials. The PID is only a sanity check, never a job identity or permit.
func peer(conn net.Conn) error {
	socket, ok := conn.(*net.UnixConn)
	if !ok {
		return ErrHeld
	}
	raw, err := socket.SyscallConn()
	if err != nil {
		return ErrHeld
	}
	var cred *unix.Xucred
	var pid int
	var inspectErr error
	err = raw.Control(func(fd uintptr) {
		cred, inspectErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if inspectErr == nil {
			pid, inspectErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		}
	})
	if err != nil || inspectErr != nil || cred == nil || cred.Version != 0 || cred.Uid != uint32(os.Geteuid()) || pid <= 0 {
		return ErrHeld
	}
	return nil
}

// Capability is limited to the exact native-tested kernel adapter. It is not
// managed-service readiness, cleanup proof, registration or launch authority.
func supported() bool {
	birth, err := n.ProcessBirth(os.Getpid())
	if err != nil {
		return false
	}
	_, err = ObserveResourceCoalition(birth, 0)
	return err == nil
}
