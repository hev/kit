//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/steer"
)

// blockingReader never returns. It closes read when a hook reads it, so a
// test learns that the hook read stdin as an event, not by a timeout.
type blockingReader struct{ read chan struct{} }

func newBlockingReader() blockingReader { return blockingReader{read: make(chan struct{})} }

func (b blockingReader) Read([]byte) (int, error) {
	close(b.read)
	select {}
}

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
	// runHook runs inside the test binary here, where a firing watchdog must
	// not end the whole run. TestHookBlockedIOFailsOpen covers the watchdog
	// in a child process.
	hookExit = func(int) {}
	t.Cleanup(func() { hookExit = os.Exit })
	return home
}

const writeCall = `{"session_id":"s1","cwd":"/w","hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"a","content":"b"}}`

// stepsAside runs runHook with stdin that never returns and reports whether
// the hook returned without reading it. Reading stdin is the event that
// fails it; there is no time window.
func stepsAside(t *testing.T, args []string, what string) {
	t.Helper()
	in := newBlockingReader()
	done := make(chan int, 1)
	var out bytes.Buffer
	go func() { done <- runHook(args, in, &out) }()
	select {
	case code := <-done:
		if code != 0 || out.Len() != 0 {
			t.Fatalf("%s: code %d out %q", what, code, out.String())
		}
	case <-in.read:
		t.Fatalf("%s still read stdin", what)
	}
}

func TestHookKillSwitchBeforeStdin(t *testing.T) {
	home := hookEnv(t)
	t.Setenv("HEV_HOOKS", "off")
	stepsAside(t, []string{"pre-tool", "--harness", "claude"}, "HEV_HOOKS=off")
	t.Setenv("HEV_HOOKS", "")
	os.MkdirAll(filepath.Join(home, ".hev"), 0700)
	os.WriteFile(filepath.Join(home, ".hev", "config.toml"), []byte("[hooks]\nenabled = false\n"), 0600)
	stepsAside(t, []string{"pre-tool", "--harness", "claude"}, "[hooks] enabled=false")
	// A settings-installed hook steps aside for a run's invocation hooks.
	os.Remove(filepath.Join(home, ".hev", "config.toml"))
	t.Setenv("HEV_HOOK_INVOCATION", "1")
	stepsAside(t, []string{"pre-tool", "--harness", "claude", "--installed-by", "kit"}, "installed hook")
	if _, err := os.Stat(filepath.Join(home, ".hev", "hooks", "events.jsonl")); err == nil {
		t.Fatal("disabled hook logged an event")
	}
}

func TestHookFailOpenOnHangingAndCrashingHandlers(t *testing.T) {
	home := hookEnv(t)
	bin := strings.Split(os.Getenv("PATH"), ":")[0]
	hevDir := filepath.Join(home, ".hev")
	os.MkdirAll(hevDir, 0700)
	warnings := func() string {
		b, _ := os.ReadFile(filepath.Join(hevDir, "hooks", "warnings.log"))
		return string(b)
	}
	var out bytes.Buffer
	// A crashing handler: the budget is far longer than the crash takes, so
	// the hook ends when the crash is reported, never on a race with the
	// deadline.
	os.WriteFile(filepath.Join(bin, "hev-hook-crash"), []byte("#!/bin/sh\necho 'segfault' >&2\nkill -KILL $$\n"), 0700)
	os.WriteFile(filepath.Join(hevDir, "config.toml"), []byte("[hooks]\ntimeout = \"10m\"\n"), 0600)
	if code := runHook([]string{"pre-tool", "--harness", "claude"}, strings.NewReader(writeCall), &out); code != 0 || out.Len() != 0 {
		t.Fatalf("a crashed handler must answer continue (no output): %d %q", code, out.String())
	}
	if w := warnings(); !strings.Contains(w, "hev-hook-crash failed") || strings.Contains(w, "did not answer") {
		t.Fatalf("warnings: %s", w)
	}
	os.Remove(filepath.Join(bin, "hev-hook-crash"))
	os.Remove(filepath.Join(hevDir, "hooks", "warnings.log"))
	// A hanging handler never answers, so only the 300ms deadline can end
	// the hook. The runner logs its warning the moment the deadline passes
	// and then returns within hooks.ReapGrace; the return is timed from that
	// logged warning, so a slow shell start before it cannot count.
	os.WriteFile(filepath.Join(bin, "hev-hook-hang"), []byte("#!/bin/sh\nsleep 60\n"), 0700)
	os.WriteFile(filepath.Join(hevDir, "config.toml"), []byte("[hooks]\ntimeout = \"300ms\"\n"), 0600)
	start := time.Now()
	if code := runHook([]string{"pre-tool", "--harness", "claude"}, strings.NewReader(writeCall), &out); code != 0 || out.Len() != 0 {
		t.Fatalf("a hung handler must answer continue (no output): %d %q", code, out.String())
	}
	returned := time.Now()
	if el := returned.Sub(start); el < 300*time.Millisecond {
		t.Fatalf("hook returned %s in, before its 300ms deadline", el)
	}
	warnedAt := warningTime(t, warnings(), "hev-hook-hang did not answer claude/pre-tool within 300ms")
	if late := returned.Sub(warnedAt); late > hooks.ReapGrace+lateAllowance {
		t.Fatalf("hook returned %s after its handler deadline passed; the bound is ReapGrace %s + %s", late, hooks.ReapGrace, lateAllowance)
	}
	events, _ := os.ReadFile(filepath.Join(hevDir, "hooks", "events.jsonl"))
	if strings.Count(string(events), `"decision":"continue"`) != 2 || strings.Contains(string(events), `"content":"b"`) {
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

// lateAllowance is how far past the product's own bound a test lets an
// event land, counted from the event that starts the bound (a deadline
// warning, the child beginning runHook), never from before it: a timer or
// goroutine wake-up, process exit, a log write, on a loaded -race run. Under
// the full race suite running alongside, the most seen on macOS and Linux
// was about 30ms, so this is more than ten times that.
const lateAllowance = 500 * time.Millisecond

// warningTime returns when the warnings.log record containing want was
// written, from the record's own timestamp.
func warningTime(t *testing.T, log, want string) time.Time {
	t.Helper()
	for _, line := range strings.Split(log, "\n") {
		var w struct {
			Time    time.Time `json:"time"`
			Warning string    `json:"warning"`
		}
		if json.Unmarshal([]byte(line), &w) == nil && strings.Contains(w.Warning, want) {
			return w.Time
		}
	}
	t.Fatalf("no warning %q in:\n%s", want, log)
	return time.Time{}
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
	grace := make(chan time.Time)
	watchdogGrace = func() <-chan time.Time { return grace }
	t.Cleanup(func() { watchdogGrace = func() <-chan time.Time { return time.After(watchdogLogGrace) } })
	warned := make(chan struct{})
	exited := make(chan int, 1)
	armWatchdog(10*time.Millisecond, func(string) { close(warned); select {} }, func(code int) { exited <- code })
	<-warned
	select {
	case code := <-exited:
		t.Fatalf("exited %d before the warning or its grace was over", code)
	default:
	}
	// The warning never finishes; the end of the grace alone lets it exit.
	grace <- time.Time{}
	if code := <-exited; code != 0 {
		t.Fatalf("exit %d", code)
	}
}

// TestHookSubprocess is not a test by itself: the blocked-I/O tests below run
// the test binary as `hev hook` through it, so the watchdog's os.Exit cannot
// end the test process. It writes hookReturned to stderr when runHook returns
// on its own. When the watchdog ends the process instead, its exit goes
// through hookExit, which first writes hookWatchdogExit and how long after
// runHook began the watchdog exited. That time is measured inside the child
// on Go's monotonic clock: a wall-clock step (an NTP correction) cannot move
// it, and process start-up and the parent's own scheduling are not in it.
func TestHookSubprocess(t *testing.T) {
	if os.Getenv("HEV_TEST_HOOK_SUBPROCESS") != "1" {
		t.Skip("helper")
	}
	began := time.Now()
	hookExit = func(code int) {
		fmt.Fprintf(os.Stderr, "%s %d\n", hookWatchdogExit, time.Since(began))
		os.Exit(code)
	}
	code := runHook([]string{"pre-tool", "--harness", "claude"}, os.Stdin, os.Stdout)
	fmt.Fprintln(os.Stderr, hookReturned)
	os.Exit(code)
}

const (
	hookWatchdogExit = "hev-test-hook-watchdog-exit"
	hookReturned     = "hev-test-hook-returned"
)

// hookRun is one `hev hook` child: its exit code, stdout, whether runHook
// returned, and, when the watchdog ended it instead, how long after runHook
// began the watchdog exited (by the child's monotonic clock).
type hookRun struct {
	code     int
	out      string
	returned bool
	watchdog bool
	ran      time.Duration
}

// runHookProcess runs `hev hook pre-tool` in a child with stdin from in (or a
// pipe left open when in is nil).
func runHookProcess(t *testing.T, home string, in *string) hookRun {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHookSubprocess$")
	// Under -race a process sleeps 1s on exit (GORACE atexit_sleep_ms) after
	// the watchdog has already ended it; turn that off so a hung hook's
	// process ends when the watchdog says, not a second later.
	cmd.Env = append(os.Environ(), "HEV_TEST_HOOK_SUBPROCESS=1", "HOME="+home, "GORACE="+strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
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
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(time.Minute):
		// Not a timing assertion: the watchdog ends the child within
		// seconds; this only keeps a broken watchdog from hanging the run.
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("hev hook still blocked after a minute; stderr %q", errb.String())
	}
	r := hookRun{out: out.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		r.code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(errb.String(), "\n") {
		if line == hookReturned {
			r.returned = true
		}
		var ns int64
		if _, err := fmt.Sscanf(line, hookWatchdogExit+" %d", &ns); err == nil {
			r.watchdog, r.ran = true, time.Duration(ns)
		}
	}
	if r.returned == r.watchdog {
		t.Fatalf("hook child neither returned nor met the watchdog, or both: stderr %q", errb.String())
	}
	return r
}

func TestHookBlockedIOFailsOpen(t *testing.T) {
	home := hookEnv(t)
	hevDir := filepath.Join(home, ".hev")
	os.MkdirAll(filepath.Join(hevDir, "hooks"), 0700)
	// Review finding 1: a FIFO config used to block before any deadline.
	// Blocking would leave the watchdog to end the process; the hook must
	// instead return on its own.
	if err := syscall.Mkfifo(filepath.Join(hevDir, "config.toml"), 0600); err != nil {
		t.Fatal(err)
	}
	call := writeCall
	if r := runHookProcess(t, home, &call); r.code != 0 || r.out != "" || !r.returned {
		t.Fatalf("FIFO config: code %d out %q returned %v", r.code, r.out, r.returned)
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
	if r := runHookProcess(t, home, &bad); r.code != 0 || r.out != "" || !r.returned {
		t.Fatalf("FIFO warnings log: code %d out %q returned %v", r.code, r.out, r.returned)
	}
	// Stdin that never closes: runHook cannot return, so the watchdog is
	// what ends the process, with continue, warnings log blocked or not. It
	// fires at the configured budget plus hookWatchdog after runHook begins,
	// and exits after at most watchdogLogGrace for its warning. Both bounds
	// count, on the child's monotonic clock, from just before runHook to the
	// watchdog's exit, so neither start-up nor a wall-clock step counts.
	budget := 100*time.Millisecond + hookWatchdog
	r := runHookProcess(t, home, nil)
	if r.code != 0 || r.out != "" || r.returned {
		t.Fatalf("hung stdin: code %d out %q returned %v", r.code, r.out, r.returned)
	}
	if r.ran < budget || r.ran > budget+watchdogLogGrace+lateAllowance {
		t.Fatalf("hung stdin: watchdog ended the hook %s after it began; want %s to %s", r.ran, budget, budget+watchdogLogGrace+lateAllowance)
	}
	t.Logf("hung stdin: exit %s after the %s deadline", r.ran-budget, budget)
}
