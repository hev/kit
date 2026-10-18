//go:build linux

package nativehost

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStaleSocketRequiresRefusalAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "socket")
	listen := func() *net.UnixListener {
		t.Helper()
		l, e := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if e != nil {
			t.Fatal(e)
		}
		l.SetUnlinkOnClose(false)
		return l
	}
	l := listen()
	dial := (&net.Dialer{}).DialContext
	if removeStaleSocket(context.Background(), path, dial) == nil {
		t.Fatal("removed live endpoint")
	}
	if _, e := os.Lstat(path); e != nil {
		t.Fatal(e)
	}
	l.Close()
	unknown := func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("unknown") }
	if removeStaleSocket(context.Background(), path, unknown) == nil {
		t.Fatal("removed unknown endpoint")
	}
	swap := func(context.Context, string, string) (net.Conn, error) {
		// Keep the old inode allocated so the replacement cannot reuse it.
		if e := os.Rename(path, path+".old"); e != nil {
			t.Fatal(e)
		}
		replacement := listen()
		defer replacement.Close()
		return nil, syscall.ECONNREFUSED
	}
	if removeStaleSocket(context.Background(), path, swap) == nil {
		t.Fatal("removed changed endpoint")
	}
	if _, e := os.Lstat(path); e != nil {
		t.Fatal(e)
	}
	if e := removeStaleSocket(context.Background(), path, dial); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatalf("stale endpoint remains: %v", e)
	}
}
