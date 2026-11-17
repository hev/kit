//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/steer"
)

// blockingReader never returns: a hook that reads stdin before checking the
// kill switch would hang the test.
type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) { select {} }

func hookEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	t.Setenv("HEV_HOOKS", "")
	t.Setenv("HEV_HOOK_INVOCATION", "")
	t.Setenv("PEERS_INBOX_TOKEN", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv(steer.EnvDir, filepath.Join(home, "steer"))
	t.Setenv(steer.EnvTarget, "dispatch:demo")
	t.Setenv("PATH", t.TempDir()+":/usr/bin:/bin")
	return home
}

const writeCall = `{"session_id":"s1","cwd":"/w","hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"a","content":"b"}}`

func TestHookKillSwitchBeforeStdin(t *testing.T) {
	home := hookEnv(t)
	done := make(chan int, 1)
	t.Setenv("HEV_HOOKS", "off")
	var out bytes.Buffer
	go func() { done <- runHook([]string{"pre-tool", "--harness", "claude"}, blockingReader{}, &out) }()
	select {
	case code := <-done:
		if code != 0 || out.Len() != 0 {
			t.Fatalf("code %d out %q", code, out.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HEV_HOOKS=off still read stdin")
	}
	t.Setenv("HEV_HOOKS", "")
	os.MkdirAll(filepath.Join(home, ".hev"), 0700)
	os.WriteFile(filepath.Join(home, ".hev", "config.toml"), []byte("[hooks]\nenabled = false\n"), 0600)
	go func() { done <- runHook([]string{"pre-tool", "--harness", "claude"}, blockingReader{}, &out) }()
	select {
	case code := <-done:
		if code != 0 || out.Len() != 0 {
			t.Fatalf("code %d out %q", code, out.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("[hooks] enabled=false still read stdin")
	}
	// A settings-installed hook steps aside for a run's invocation hooks.
	os.Remove(filepath.Join(home, ".hev", "config.toml"))
	t.Setenv("HEV_HOOK_INVOCATION", "1")
	go func() {
		done <- runHook([]string{"pre-tool", "--harness", "claude", "--installed-by", "kit"}, blockingReader{}, &out)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("installed hook did not step aside")
	}
	if _, err := os.Stat(filepath.Join(home, ".hev", "hooks", "events.jsonl")); err == nil {
		t.Fatal("disabled hook logged an event")
	}
}

func TestHookFailOpenOnHangingAndCrashingHandlers(t *testing.T) {
	home := hookEnv(t)
	bin := strings.Split(os.Getenv("PATH"), ":")[0]
	os.WriteFile(filepath.Join(bin, "hev-hook-hang"), []byte("#!/bin/sh\nsleep 60\n"), 0700)
	os.WriteFile(filepath.Join(bin, "hev-hook-crash"), []byte("#!/bin/sh\necho 'segfault' >&2\nkill -KILL $$\n"), 0700)
	os.MkdirAll(filepath.Join(home, ".hev"), 0700)
	os.WriteFile(filepath.Join(home, ".hev", "config.toml"), []byte("[hooks]\ntimeout = \"300ms\"\n"), 0600)
	var out bytes.Buffer
	start := time.Now()
	if code := runHook([]string{"pre-tool", "--harness", "claude"}, strings.NewReader(writeCall), &out); code != 0 {
		t.Fatal(code)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("hook took %s", time.Since(start))
	}
	if out.Len() != 0 {
		t.Fatalf("failed handlers must answer continue (no output): %s", out.String())
	}
	warnings, _ := os.ReadFile(filepath.Join(home, ".hev", "hooks", "warnings.log"))
	if !strings.Contains(string(warnings), "hev-hook-hang did not answer") || !strings.Contains(string(warnings), "hev-hook-crash failed") {
		t.Fatalf("warnings: %s", warnings)
	}
	events, _ := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
	if !strings.Contains(string(events), `"decision":"continue"`) || strings.Contains(string(events), `"content":"b"`) {
		t.Fatalf("event log: %s", events)
	}
	// Garbage input also continues.
	out.Reset()
	if code := runHook([]string{"pre-tool", "--harness", "claude"}, strings.NewReader("{oops"), &out); code != 0 || out.Len() != 0 {
		t.Fatalf("%d %q", code, out.String())
	}
	if code := runHook([]string{"pre-tool", "--harness", "pi"}, strings.NewReader(writeCall), &out); code != 0 || out.Len() != 0 {
		t.Fatalf("%d %q", code, out.String())
	}
}

func TestHookUrgentBlocksWriteNatively(t *testing.T) {
	home := hookEnv(t)
	s := steer.Store{Dir: filepath.Join(home, "steer"), Target: "dispatch:demo"}
	m, err := s.Enqueue(steer.Urgent, "scope changed", "foh", steer.ToolBoundary, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	runHook([]string{"pre-tool", "--harness", "claude"}, strings.NewReader(writeCall), &out)
	var v struct {
		HookSpecificOutput struct {
			HookEventName, PermissionDecision, PermissionDecisionReason string
		}
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("%s: %v", out.String(), err)
	}
	if v.HookSpecificOutput.HookEventName != "PreToolUse" || v.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(v.HookSpecificOutput.PermissionDecisionReason, m.ID) || !strings.Contains(v.HookSpecificOutput.PermissionDecisionReason, "steer ack") {
		t.Fatalf("%+v", v)
	}
	events, _ := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
	if !strings.Contains(string(events), `"decision":"block"`) || !strings.Contains(string(events), `"target":"dispatch:demo"`) {
		t.Fatalf("%s", events)
	}
}
