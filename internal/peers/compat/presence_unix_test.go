//go:build !windows

package compat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalPeerInventoryRefusesFailedSamples(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", home)
	t.Setenv("HEV_PEERS_STATE", filepath.Join(home, "peers"))
	for _, tc := range []struct {
		name, ps, tmux string
		fail           bool
	}{
		{"no server", "echo '1 0 init'", "echo 'no server running on private-socket' >&2; exit 1", false},
		{"ps failed", "echo '1 0 init'; exit 23", "exit 0", true},
		{"ps empty", "exit 0", "exit 0", true},
		{"tmux failed", "echo '1 0 init'", "exit 23", true},
		{"ps timed out", "exec /bin/sleep 60", "exit 0", true},
		{"tmux timed out", "echo '1 0 init'", "exec /bin/sleep 60", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for name, body := range map[string]string{"ps": tc.ps, "tmux": tc.tmux} {
				if err := os.WriteFile(filepath.Join(home, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			timeout := 5 * time.Second
			if tc.ps == "exec /bin/sleep 60" || tc.tmux == "exec /bin/sleep 60" {
				timeout = 500 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			peers, err := LocalPeers(ctx)
			if tc.fail && (err == nil || peers != nil) || !tc.fail && (err != nil || peers == nil || len(peers) > 0) {
				t.Fatalf("sample peers=%v, err=%v", peers, err)
			}
		})
	}
}
