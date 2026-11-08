package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPeersAliasIgnoresBackend(t *testing.T) {
	old := os.Args
	os.Args = []string{"peers"}
	defer func() { os.Args = old }()
	t.Setenv("HEV_PEERS_BACKEND", "peers")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TMUX_TMPDIR", home)
	t.Setenv("TMUX_PANE", "")
	c := newPeersCommand()
	var out, errout bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errout)
	// The reference-compatible CLI rejects the limit itself, without executing the override.
	c.SetArgs([]string{"inbox", "--session", "desk", "--peek", "--limit", "0"})
	err := c.Execute()
	if exit, ok := err.(*commandExit); !ok || exit.code != 1 || !strings.Contains(errout.String(), "peers: --limit must be 1..64") {
		t.Fatalf("%v: %s", err, errout.String())
	}
}

func TestPeersInboxOnlyUsesNativeEngine(t *testing.T) {
	if !nativePeersEngine([]string{"send", "x", "y", "--inbox-only"}) {
		t.Fatal("--inbox-only must select the native engine")
	}
	t.Setenv("HEV_PEERS_ENGINE", "native")
	if !nativePeersEngine([]string{"ls"}) {
		t.Fatal("HEV_PEERS_ENGINE=native must select the native engine")
	}
	t.Setenv("HEV_PEERS_ENGINE", "")
	if nativePeersEngine([]string{"send", "x", "y"}) {
		t.Fatal("plain sends use the reference-compatible CLI")
	}
}
