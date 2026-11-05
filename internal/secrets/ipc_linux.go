package secrets

import (
	"context"
	"fmt"
	"github.com/hev/kit/internal/platformops"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
)

func endpoint() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hev", "secrets", "agent.sock")
}
func listen() (net.Listener, error) {
	p := endpoint()
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm() != 0700 {
		return nil, fmt.Errorf("secrets directory must be private")
	}
	release, err := platformops.LockPath(filepath.Join(dir, "agent.lock"), true)
	if err != nil {
		return nil, fmt.Errorf("secrets endpoint already running")
	}
	success := false
	defer func() {
		if !success {
			release()
		}
	}()
	if c, e := net.Dial("unix", p); e == nil {
		c.Close()
		return nil, fmt.Errorf("secrets endpoint already running")
	}
	os.Remove(p)
	l, err := net.Listen("unix", p)
	if err == nil {
		err = os.Chmod(p, 0600)
		if err != nil {
			l.Close()
		}
	}
	if err != nil {
		return nil, err
	}
	success = true
	return &lockedListener{Listener: l, release: release}, nil
}
func dial(ctx context.Context) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint())
}
func sameUser(c net.Conn) bool { return socketUser(c, uint32(os.Getuid())) }
func socketUser(c net.Conn, expected uint32) bool {
	u, ok := c.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := u.SyscallConn()
	if err != nil {
		return false
	}
	valid := false
	err = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		valid = e == nil && cred.Uid == expected
	})
	return err == nil && valid
}

func serverUser(c net.Conn) bool { return sameUser(c) }
