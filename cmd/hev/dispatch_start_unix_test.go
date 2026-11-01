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

func TestDispatchTargetRepoRefusalOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "sender.toml")
	target := filepath.Join(filepath.Dir(cfg), "target.toml")
	// A stale sender has no policy. The target still refuses before reading Git.
	if err := os.WriteFile(cfg, []byte("[hosts.here]\nlocal=true\ncap=100000\nconfig_path='target.toml'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("[hosts.here]\nlocal=true\ncap=100000\n[hosts.here.admission]\nrepos=[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	order := filepath.Join(t.TempDir(), "order.md")
	if err := os.WriteFile(order, []byte("---\nname: repo-refused\nrepo: project\nhost: here\n---\n## Acceptance\n- Done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := newDispatchCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfg, order})
	err := cmd.Execute()
	if err == nil || err.Error() != `Error: repo "project" is not admitted on host "here"` || out.Len() != 0 {
		t.Fatalf("output %q, error %v", out.String(), err)
	}
	// main prints the returned error once and exits 1; Cobra must add no usage/error.
	list := newDispatchCommand()
	list.SetOut(&out)
	list.SetErr(&out)
	list.SetArgs([]string{"--config", cfg, "ls", "--all"})
	if err := list.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `repo "project" is not admitted on host "here"`) {
		t.Fatal(out.String())
	}
}
