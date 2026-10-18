//go:build darwin

package nativehost

import (
	"context"
	"net"
	"os"
	"path/filepath"
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

func TestDarwinTransportDoesNotEnableExecutionRole(t *testing.T) {
	first, second := net.Pipe()
	defer first.Close()
	defer second.Close()
	if peer(first) == nil {
		t.Fatal("non-Unix transport accepted")
	}
	if supported() {
		t.Fatal("credential adapter enabled unproved lifecycle role")
	}
	if RunConfig(context.Background(), "/nonexistent/config") == nil {
		t.Fatal("Darwin managed role enabled")
	}
}
