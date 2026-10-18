//go:build linux || darwin

package nativehost

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func unixPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	root, err := os.MkdirTemp("", "nf-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	path := filepath.Join(root, "socket")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := listener.AcceptUnix()
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); server.Close() })
	return client, server
}

func TestUnixRequestFramingAndBounds(t *testing.T) {
	client, server := unixPair(t)
	payload := []byte(`{"version":"hevd-native-dispatch/v1","profile_id":"one","descriptor_ref":"/owned/ref","descriptor_digest":"digest"}` + "\n")
	done := make(chan error, 1)
	go func() { done <- writeRequest(client, payload) }()
	got, err := readRequest(server)
	if err != nil || string(got) != string(payload) {
		t.Fatal("request bytes changed", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var call Dispatch
	if strict(got, &call) != nil {
		t.Fatal("valid strict request rejected")
	}
	client2, server2 := unixPair(t)
	go func() {
		_, err := client2.Write([]byte(strings.Repeat("x", maxFrame+1)))
		_ = client2.CloseWrite()
		done <- err
	}()
	if _, err = readRequest(server2); err == nil {
		t.Fatal("oversized request accepted")
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if writeRequest(client2, make([]byte, maxFrame+1)) == nil {
		t.Fatal("oversized outbound request accepted")
	}
	client3, server3 := unixPair(t)
	go func() { done <- writeRequest(client3, append(payload, []byte("{}")...)) }()
	got, err = readRequest(server3)
	if err != nil {
		t.Fatal(err)
	}
	if strict(got, &call) == nil {
		t.Fatal("trailing JSON accepted")
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestUnixEndpointOwnsLockUntilClose(t *testing.T) {
	f := setup(t)
	cfg, err := VerifyConfig(f.config)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("", "nl-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	cfg.Socket = filepath.Join(root, "socket")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(f.config, data, 0600); err != nil {
		t.Fatal(err)
	}
	first, err := openEndpoint(context.Background(), f.config, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if verifyListener(first) != nil {
		t.Fatal("owned listener rejected")
	}
	if second, err := openEndpoint(context.Background(), f.config, cfg); err == nil {
		second.Close()
		t.Fatal("second owner acquired live endpoint")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := openEndpoint(context.Background(), f.config, cfg)
	if err != nil {
		t.Fatal("owner lock not released", err)
	}
	second.Close()
}
