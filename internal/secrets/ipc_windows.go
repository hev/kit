package secrets

import (
	"context"
	"net"

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
func endpoint() string { s, _ := currentSID(); return `\\.\pipe\hev-secrets-` + s }
func listen() (net.Listener, error) {
	s, e := currentSID()
	if e != nil {
		return nil, e
	}
	return winio.ListenPipe(endpoint(), &winio.PipeConfig{SecurityDescriptor: pipeSDDL(s)})
}
func dial(ctx context.Context) (net.Conn, error) { return winio.DialPipeContext(ctx, endpoint()) }

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
