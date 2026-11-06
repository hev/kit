package secrets

import (
	"golang.org/x/sys/unix"
	"net"
	"os"
	"testing"
)

func TestOtherUserRefused(t *testing.T) {
	fds, e := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer unix.Close(fds[1])
	f := os.NewFile(uintptr(fds[0]), "peer")
	c, e := net.FileConn(f)
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if !sameUser(c) {
		t.Fatal("same user refused")
	}
	if socketUser(c, uint32(os.Getuid())+1) {
		t.Fatal("other UID accepted")
	}
}
