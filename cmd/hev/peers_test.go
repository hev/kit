package main

import (
	"bytes"
	"os"
	"testing"
)

func TestPeersAliasIgnoresBackend(t *testing.T) {
	old := os.Args
	os.Args = []string{"peers"}
	defer func() { os.Args = old }()
	t.Setenv("HEV_PEERS_BACKEND", "peers")
	c := newPeersCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	// Unsupported native operation returns immediately, without executing the override.
	c.SetArgs([]string{"inbox", "--limit", "0"})
	if err := c.Execute(); err == nil || err.Error() != "--limit must be positive" {
		t.Fatalf("%v", err)
	}
}
