package secrets

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func currentSID() (string, error) {
	t, e := windows.OpenCurrentProcessToken()
	if e != nil {
		return "", e
	}
	defer t.Close()
	u, e := t.GetTokenUser()
	if e != nil {
		return "", e
	}
	return u.User.Sid.String(), nil
}
func defaultEndpoint() string { s, _ := currentSID(); return pipePrefix + "hev-secrets-" + s }

const pipePrefix = `\\.\pipe\`

// validEndpoint accepts a local named pipe, \\.\pipe\NAME. An override keeps
// the same protected DACL and peer-token checks as the default pipe.
func validEndpoint(p string) error {
	if len(p) <= len(pipePrefix) || !strings.EqualFold(p[:len(pipePrefix)], pipePrefix) || strings.ContainsAny(p[len(pipePrefix):], `\/`) {
		return fmt.Errorf("%s must be a local pipe, %sNAME", EndpointEnv, pipePrefix)
	}
	return nil
}
func listen() (net.Listener, error) {
	p := Endpoint()
	if err := validEndpoint(p); err != nil {
		return nil, err
	}
	s, e := currentSID()
	if e != nil {
		return nil, e
	}
	return winio.ListenPipe(p, &winio.PipeConfig{SecurityDescriptor: pipeSDDL(s)})
}
func dial(ctx context.Context) (net.Conn, error) { return winio.DialPipeContext(ctx, Endpoint()) }

// Both ends verify the peer process token as well as the protected pipe DACL.
func sameUser(c net.Conn) bool   { return peerUser(c, false) }
func serverUser(c net.Conn) bool { return peerUser(c, true) }
func peerUser(c net.Conn, server bool) bool {
	f, ok := c.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	var pid uint32
	var err error
	if server {
		err = windows.GetNamedPipeServerProcessId(windows.Handle(f.Fd()), &pid)
	} else {
		err = windows.GetNamedPipeClientProcessId(windows.Handle(f.Fd()), &pid)
	}
	if err != nil {
		return false
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(p)
	var t windows.Token
	if windows.OpenProcessToken(p, windows.TOKEN_QUERY, &t) != nil {
		return false
	}
	defer t.Close()
	u, err := t.GetTokenUser()
	if err != nil {
		return false
	}
	s, err := currentSID()
	return err == nil && u.User.Sid.String() == s
}

func pipeSDDL(sid string) string { return "D:P(A;;GA;;;" + sid + ")" }
