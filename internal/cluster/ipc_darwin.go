package cluster

import (
	"context"
	"fmt"
	"github.com/hev/kit/internal/platformops"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func defaultEndpoint() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hev", "cluster", "agent.sock")
}

// validEndpoint accepts an absolute socket path.
func validEndpoint(p string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s must be an absolute socket path", EndpointEnv)
	}
	return nil
}

// lockPath sits beside the socket: agent.sock locks agent.lock, so each
// endpoint has its own lock and the default is unchanged.
func lockPath(p string) string {
	return filepath.Join(filepath.Dir(p), strings.TrimSuffix(filepath.Base(p), ".sock")+".lock")
}
func listen() (net.Listener, error) {
	p := Endpoint()
	if err := validEndpoint(p); err != nil {
		return nil, err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm() != 0700 {
		return nil, fmt.Errorf("cluster directory must be private")
	}
	release, err := platformops.LockPath(lockPath(p), true)
	if err != nil {
		return nil, fmt.Errorf("cluster endpoint already running")
	}
	success := false
	defer func() {
		if !success {
			release()
		}
	}()
	if c, e := net.Dial("unix", p); e == nil {
		c.Close()
		return nil, fmt.Errorf("cluster endpoint already running")
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
	return (&net.Dialer{}).DialContext(ctx, "unix", Endpoint())
}
func sameUser(c net.Conn) bool {
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
		cred, e := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		valid = e == nil && cred.Uid == uint32(os.Getuid())
	})
	return err == nil && valid
}

func serverUser(c net.Conn) bool { return sameUser(c) }
