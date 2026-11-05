package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsDispatchPauseCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.worker]\nlocal=true\ncap=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) error {
		cmd := newDispatchCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"--config", cfg}, args...))
		return cmd.Execute()
	}
	if err := run("pause"); err == nil || !strings.Contains(err.Error(), "--host is required") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "dispatch")); !os.IsNotExist(err) {
		t.Fatal("missing host changed state")
	}
	if err := run("pause", "--host", "worker"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "dispatch", ".paused")); err != nil {
		t.Fatal(err)
	}
	if err := run("resume", "--host", "worker", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "dispatch", ".paused")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
