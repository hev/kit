//go:build !windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

func TestDispatchStartSharedCapacityOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho d-busy\n"), 0700)
	cfg := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(cfg, []byte("[hosts.here]\nlocal = true\ncap = 1\n"), 0600)
	order := filepath.Join(t.TempDir(), "order.md")
	os.WriteFile(order, []byte("---\nname: sample\nrepo: sample\nhost: here\n---\nTask\n## Acceptance\n- Done\n"), 0600)
	for _, args := range [][]string{{"--config", cfg, order}, {"--config", cfg, "--host", "here", "run", "sample", "task", "--name", "sample"}} {
		cmd := newDispatchCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "host full") || !strings.Contains(out.String(), "here: 0 free") {
			t.Fatalf("%v: %s %v", args, out.String(), err)
		}
	}
}

func TestRunRejectsInventoryExplicitName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	os.MkdirAll(filepath.Join(dispatch.Root(), "taken"), 0700)
	cfg := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(cfg, []byte("[hosts.here]\nlocal = true\ncap = 1\n"), 0600)
	cmd := newDispatchCommand()
	cmd.SetArgs([]string{"--config", cfg, "run", "sample", "task", "--name", "taken"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "name already exists") {
		t.Fatalf("%v %s", err, out.String())
	}
}

func TestSenderDefersWorkspaceToTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	cfg := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(cfg, []byte("workspace='/sender-workspace'\n[hosts.here]\nlocal=true\ncap=1\nconfig_path='target.toml'\n"), 0600)
	os.WriteFile(filepath.Join(filepath.Dir(cfg), "target.toml"), []byte("workspace='/target-workspace'\n"), 0600)
	order := filepath.Join(t.TempDir(), "order.md")
	os.WriteFile(order, []byte("---\nname: workspace-fallback\nrepo: missing\nhost: here\n---\nTask\n## Acceptance\n- Done\n"), 0600)
	cmd := newDispatchCommand()
	cmd.SetArgs([]string{"--config", cfg, order})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "/target-workspace/missing") {
		t.Fatal(err)
	}
}
