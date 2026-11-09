//go:build !windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

func TestDispatchStartSharedCapacityOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho d-busy\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.here]\nlocal = true\ncap = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	order := filepath.Join(t.TempDir(), "order.md")
	if err := os.WriteFile(order, []byte("---\nname: sample\nrepo: sample\nhost: here\n---\nTask\n## Acceptance\n- Done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--config", cfg, order}, {"--config", cfg, "--host", "here", "run", "sample", "task", "--name", "sample"}} {
		cmd := newDispatchCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err != nil || !strings.Contains(out.String(), "queued sample (position 1)") {
			t.Fatalf("%v: %s %v", args, out.String(), err)
		}
		if _, err := dispatch.Handle(dispatch.Request{Action: "stop", Name: "sample"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunRejectsInventoryExplicitName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dispatch.Root(), "taken"), 0700)
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.here]\nlocal = true\ncap = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("workspace='/sender-workspace'\n[hosts.here]\nlocal=true\ncap=1\nconfig_path='target.toml'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfg), "target.toml"), []byte("workspace='/target-workspace'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	order := filepath.Join(t.TempDir(), "order.md")
	if err := os.WriteFile(order, []byte("---\nname: workspace-fallback\nrepo: missing\nhost: here\n---\nTask\n## Acceptance\n- Done\n"), 0600); err != nil {
		t.Fatal(err)
	}
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
	t.Setenv("HEV_DISPATCH_CONFIG", target)
	// A stale sender has no policy. The target still refuses before reading Git.
	if err := os.WriteFile(cfg, []byte("[hosts.here]\nlocal=true\ncap=100000\nconfig_path='target.toml'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("[hosts.here]\nlocal=true\ncap=100000\n[admission]\nrepos=[]\n"), 0600); err != nil {
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

func TestDispatchListErrorFirstLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "sender.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.here]\nlocal=true\ncap=100000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(dispatch.Root(), "example")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := dispatch.WriteJSON(filepath.Join(dir, "meta.json"), dispatch.Meta{Order: dispatch.Order{Name: "example"}}); err != nil {
		t.Fatal(err)
	}
	if err := dispatch.WriteJSON(filepath.Join(dir, "result.json"), dispatch.Result{ExitCode: 1, Error: "first line\nsecond line"}); err != nil {
		t.Fatal(err)
	}
	cmd := newDispatchCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfg, "ls", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "first line") || strings.Contains(out.String(), "second line") {
		t.Fatal(out.String())
	}
}

func TestDispatchRelaunchRefusalOutput(t *testing.T) {
	for _, action := range []string{"rework", "say"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			bin := t.TempDir()
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			cfg := filepath.Join(t.TempDir(), "target.toml")
			t.Setenv("HEV_DISPATCH_CONFIG", cfg)
			if err := os.WriteFile(cfg, []byte("[hosts.here]\nlocal=true\ncap=100\n[admission]\nrepos=[]\n"), 0600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(dispatch.Root(), "example")
			if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := dispatch.WriteJSON(filepath.Join(dir, "meta.json"), dispatch.Meta{Order: dispatch.Order{Name: "example", Repo: "project", Host: "here"}}); err != nil {
				t.Fatal(err)
			}
			cmd := newDispatchCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--config", cfg, action, "example", "try again"})
			err := cmd.Execute()
			if err == nil || !strings.HasPrefix(err.Error(), "Error: ") || !strings.Contains(err.Error(), "not admitted") || out.Len() != 0 {
				t.Fatalf("error %v, output %q", err, out.String())
			}
		})
	}
}

func TestDispatchHostsReportsTargetCap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.toml")
	if err := os.WriteFile(target, []byte("cap=3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", target)
	sender := filepath.Join(t.TempDir(), "sender.toml")
	if err := os.WriteFile(sender, []byte("[hosts.here]\nlocal=true\ncap=100\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := newDispatchCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", sender, "hosts"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if fields := strings.Fields(out.String()); len(fields) != 14 || fields[8] != "3" || fields[10] != "3" || fields[12] != "-" {
		t.Fatal(out.String())
	}
	// Undelivered owner notices on the host show in NOTICES.
	t.Setenv("PEERS_SELF", "build-1")
	if _, err := dispatch.ApplyOrchestrator(strings.Replace(pullSpec, "notify_route: pull", "notify_route: pull\nhost: here", 1), false); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	cmd = newDispatchCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", sender, "hosts"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`yes\s+1 \(oldest 0s\)`).MatchString(out.String()) {
		t.Fatal(out.String())
	}
}
