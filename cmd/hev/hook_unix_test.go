//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hev/kit/internal/steer"
	"github.com/hev/kit/internal/testenv"
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
	os.WriteFile(filepath.Join(bin, "hev-hook-hang"), []byte("#!/bin/sh\n[ \"$1\" = "+testenv.PrimeArg+" ] && exit 0\nsleep 60\n"), 0700)
	os.WriteFile(filepath.Join(bin, "hev-hook-crash"), []byte("#!/bin/sh\necho 'segfault' >&2\nkill -KILL $$\n"), 0700)
	// The 300ms deadline is for answering, not for macOS's first-run check.
	testenv.PrimeExec(t, nil, filepath.Join(bin, "hev-hook-hang"), filepath.Join(bin, "hev-hook-crash"))
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

func TestArmWatchdogExitsEvenWhenLoggingBlocks(t *testing.T) {
	exited := make(chan int, 1)
	start := time.Now()
	armWatchdog(10*time.Millisecond, func(string) { select {} }, func(code int) { exited <- code })
	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		if el := time.Since(start); el > time.Second {
			t.Fatalf("exit took %s", el)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a blocked warning log kept the watchdog from exiting")
	}
}

// TestHookSubprocess is not a test by itself: the blocked-I/O tests below run
// the test binary as `hev hook` through it, so the watchdog's os.Exit cannot
// end the test process.
func TestHookSubprocess(t *testing.T) {
	if os.Getenv("HEV_TEST_HOOK_SUBPROCESS") != "1" {
		t.Skip("helper")
	}
	os.Exit(runHook([]string{"pre-tool", "--harness", "claude"}, os.Stdin, os.Stdout))
}

// runHookProcess runs `hev hook pre-tool` in a child with stdin from in (or a
// pipe left open when in is nil) and returns its exit code, output and time.
func runHookProcess(t *testing.T, home string, in *string) (int, string, time.Duration) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHookSubprocess$")
	cmd.Env = append(os.Environ(), "HEV_TEST_HOOK_SUBPROCESS=1", "HOME="+home)
	var out bytes.Buffer
	cmd.Stdout = &out
	if in != nil {
		cmd.Stdin = strings.NewReader(*in)
	} else {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		defer r.Close()
		cmd.Stdin = r
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return code, out.String(), time.Since(start)
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("hev hook still blocked after 15s")
	}
	return 0, "", 0
}

func TestHookBlockedIOFailsOpen(t *testing.T) {
	home := hookEnv(t)
	hevDir := filepath.Join(home, ".hev")
	os.MkdirAll(filepath.Join(hevDir, "hooks"), 0700)
	// Review finding 1: a FIFO config used to block before any deadline.
	if err := syscall.Mkfifo(filepath.Join(hevDir, "config.toml"), 0600); err != nil {
		t.Fatal(err)
	}
	call := writeCall
	code, out, el := runHookProcess(t, home, &call)
	if code != 0 || out != "" || el > 5*time.Second {
		t.Fatalf("FIFO config: code %d out %q in %s", code, out, el)
	}
	warnings, _ := os.ReadFile(filepath.Join(hevDir, "hooks", "warnings.log"))
	if !strings.Contains(string(warnings), "not a regular file") {
		t.Fatalf("warnings: %s", warnings)
	}
	// A FIFO warnings log with malformed input (which warns) used to block.
	os.Remove(filepath.Join(hevDir, "config.toml"))
	os.WriteFile(filepath.Join(hevDir, "config.toml"), []byte("[hooks]\ntimeout = \"100ms\"\n"), 0600)
	os.Remove(filepath.Join(hevDir, "hooks", "warnings.log"))
	if err := syscall.Mkfifo(filepath.Join(hevDir, "hooks", "warnings.log"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := "{oops"
	code, out, el = runHookProcess(t, home, &bad)
	if code != 0 || out != "" || el > 5*time.Second {
		t.Fatalf("FIFO warnings log: code %d out %q in %s", code, out, el)
	}
	// Stdin that never closes: the watchdog ends the process at the
	// configured budget plus hookWatchdog, warnings log blocked or not.
	code, out, el = runHookProcess(t, home, nil)
	if code != 0 || out != "" || el > 100*time.Millisecond+hookWatchdog+2*time.Second {
		t.Fatalf("hung stdin: code %d out %q in %s", code, out, el)
	}
}
