//go:build darwin

package nativehost

import (
	"context"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDarwinKernelPeerCredentialsBothDirections(t *testing.T) {
	root, err := os.MkdirTemp("", "np-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	path := filepath.Join(root, "peer.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if peer(client) != nil || peer(server) != nil {
		t.Fatal("kernel-authenticated local Unix peers rejected")
	}
	server.Close()
	if peer(server) == nil {
		t.Fatal("closed descriptor accepted")
	}
}

func TestDarwinTransportRequiresPinnedConfigAndTestedKernelCapability(t *testing.T) {
	first, second := net.Pipe()
	defer first.Close()
	defer second.Close()
	if peer(first) == nil {
		t.Fatal("non-Unix transport accepted")
	}
	build, e := unix.Sysctl("kern.osversion")
	want := e == nil && runtime.GOARCH == "arm64" && build == "25G83"
	if supported() != want {
		t.Fatal("kernel capability probe did not match native-tested build", build)
	}
	if RunConfig(context.Background(), "/nonexistent/config") == nil {
		t.Fatal("unconfigured Darwin managed role accepted")
	}
}
