//go:build !windows

package dispatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSupervisorStopsDescendantsAndPreservesNotifyFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := "#!/bin/sh\necho '{\"type\":\"thread.started\",\"thread_id\":\"test-session\"}'\nsleep 60 &\necho $! > ../child.pid\nwait\n"
	if e := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	// Keep optional PR discovery isolated from the network.
	if e := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0700); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(Root(), "test-stop")
	if e := os.MkdirAll(filepath.Join(dir, "wt"), 0700); e != nil {
		t.Fatal(e)
	}
	config := filepath.Join(bin, "dispatch.toml")
	if e := os.WriteFile(config, []byte("notify = [\"missing-notify-command\"]\n"), 0600); e != nil {
		t.Fatal(e)
	}
	m := Meta{LaunchResolved: true, ConfigPath: config, Order: Order{Name: "test-stop", Harness: "codex", Timeout: 1}, Branch: "d/test-stop"}
	if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-finished:
			return
		default:
		}
		_ = os.WriteFile(filepath.Join(dir, "stop"), nil, 0600)
		select {
		case <-finished:
		case <-time.After(30 * time.Second):
			_ = stopHarness(dir)
			t.Error("supervisor cleanup did not finish")
		}
	})
	go func() { defer close(finished); done <- Supervise("test-stop") }()
	deadline := time.Now().Add(30 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("supervisor ended before child publication: %v", err)
		default:
		}
		b, e := os.ReadFile(filepath.Join(dir, "child.pid"))
		if e == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			if pid > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child did not start")
	}
	if e := os.WriteFile(filepath.Join(dir, "stop"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("supervisor did not stop")
	}
	var r Result
	if e := readJSON(filepath.Join(dir, "result.json"), &r); e != nil {
		t.Fatal(e)
	}
	if r.ExitCode != 143 || !r.Stopped || r.Session != "test-session" || r.NotifyError == "" || State(&r, false) != "failed" {
		t.Fatalf("%+v", r)
	}
	// On Linux an orphan zombie may remain until init reaps it; it cannot execute.
	if e := syscall.Kill(pid, 0); e == nil {
		b, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
		if !strings.Contains(string(b), ") Z ") {
			t.Fatalf("descendant %d remains live", pid)
		}
	}
}

func TestAdmissionRefusesBeforeCreatingRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if e := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho d-existing\n"), 0700); e != nil {
		t.Fatal(e)
	}
	_, e := Handle(Request{Action: "start", Name: "example", Order: good, Cap: 1})
	if e == nil || e.Error() != "host full (1/1)" {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(Root(), "example")); !os.IsNotExist(e) {
		t.Fatalf("refusal created a record: %v", e)
	}
}

func TestAdmissionReleasesLockAndFailureFreesReservation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	gate := filepath.Join(bin, "gate")
	os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\ntouch '"+gate+"'\nwhile [ ! -f '"+gate+".release' ]; do sleep 0.01; done\nexit 1\n"), 0700)
	done := make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		_ = os.WriteFile(gate+".release", nil, 0600)
		select {
		case <-finished:
		case <-time.After(30 * time.Second):
			t.Error("setup cleanup did not finish")
		}
	})
	go func() {
		defer close(finished)
		_, e := Handle(Request{Action: "start", Name: "example", Order: good, Meta: Meta{Order: Order{Name: "example"}}, Cap: 1})
		done <- e
	}()
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case err := <-done:
			t.Fatalf("setup ended before gate publication: %v", err)
		default:
		}
		if _, e := os.Stat(gate); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("setup not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
	second := make(chan error, 1)
	go func() {
		_, err := Handle(Request{Action: "start", Name: "second", Order: strings.Replace(good, "example", "second", 1), Meta: Meta{Order: Order{Name: "second"}}, Cap: 1})
		second <- err
	}()
	select {
	case err := <-second:
		if err == nil || err.Error() != "host full (1/1)" {
			t.Fatalf("reservation/lock: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("admission lock held across setup")
	}
	os.WriteFile(gate+".release", nil, 0600)
	if err := <-done; err == nil {
		t.Fatal("expected failed git")
	}
	var result Result
	if err := readJSON(filepath.Join(Root(), "example", "result.json"), &result); err != nil || State(&result, false) != "failed" {
		t.Fatalf("%+v %v", result, err)
	}
	live := map[string]bool{}
	if err := addReservations(live); err != nil || len(live) != 0 {
		t.Fatalf("slot leaked: %v %v", live, err)
	}
}

func TestRPCUsesStdinForLargeOrders(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	input := filepath.Join(bin, "request")
	os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\nfor arg do case \"$arg\" in *PAYLOAD*) exit 2;; esac; done\ncat > '"+input+"'\necho '{}'\n"), 0700)
	raw := strings.Repeat("PAYLOAD", 40000)
	if _, err := RPC(context.Background(), Host{SSH: "buildbox"}, Request{Action: "start", Order: raw}); err != nil {
		t.Fatal(err)
	}
	var q Request
	if err := readJSON(input, &q); err != nil || q.Order != raw {
		t.Fatalf("stdin request lost: %v", err)
	}
}

func TestLogsOffset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	dir := filepath.Join(Root(), "example")
	os.MkdirAll(dir, 0700)
	WriteJSON(filepath.Join(dir, "meta.json"), Meta{})
	os.WriteFile(filepath.Join(dir, "log"), []byte("first\nsecond\n"), 0600)
	r, err := Handle(Request{Action: "logs", Name: "example", Offset: 6})
	if err != nil || r.Log != "second\n" || r.Offset != 13 || r.Active {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestEnvironmentNamesRecordedAtAdmission(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPENAI_API_KEY", "fresh-value")
	m := Meta{Order: Order{Host: "here"}}
	resolveLaunch(&m, Config{Hosts: map[string]Host{"here": {Env: map[string]string{"CUSTOM_AUTH": "configured-value"}}}})
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "changed-value")
	values, err := launchEnvironment(dir)
	if err != nil {
		t.Fatal(err)
	}
	env := strings.Join(values, "\n")
	if strings.Contains(env, "CUSTOM_AUTH=configured-value") || !strings.Contains(env, "OPENAI_API_KEY=changed-value") {
		t.Fatal("missing recorded environment")
	}
}

func TestSignalHelper(t *testing.T) {
	if os.Getenv("DISPATCH_SIGNAL_HELPER") != "1" {
		return
	}
	if err := Supervise("signal-test"); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestSupervisorSignalAndFallbackStop(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGKILL} {
		t.Run(sig.String(), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			bin := t.TempDir()
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\necho '{\"thread_id\":\"signal-session\"}'\nsleep 60 &\necho $! > ../child.pid\nwait\n"), 0700)
			os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0700)
			os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700)
			dir := filepath.Join(Root(), "signal-test")
			os.MkdirAll(filepath.Join(dir, "wt"), 0700)
			cfg := filepath.Join(bin, "config.toml")
			os.WriteFile(cfg, []byte("notify=[]\n"), 0600)
			WriteJSON(filepath.Join(dir, "meta.json"), Meta{LaunchResolved: true, ConfigPath: cfg, Order: Order{Name: "signal-test", Harness: "codex", Timeout: 1}})
			c := exec.Command(os.Args[0], "-test.run=^TestSignalHelper$")
			c.Env = append(os.Environ(), "DISPATCH_SIGNAL_HELPER=1")
			if err := c.Start(); err != nil {
				t.Fatal(err)
			}
			defer c.Process.Kill()
			deadline := time.Now().Add(30 * time.Second)
			var pid int
			ready := false
			for time.Now().Before(deadline) {
				b, _ := os.ReadFile(filepath.Join(dir, "child.pid"))
				pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
				if pid > 0 {
					var h pidRecord
					if readJSON(filepath.Join(dir, "harness.json"), &h) == nil {
						ready = true
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !ready {
				t.Fatal("child and harness not published")
			}
			if err := c.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- c.Wait() }()
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("signal did not terminate supervisor")
			}
			if sig == syscall.SIGKILL {
				if _, err := Handle(Request{Action: "stop", Name: "signal-test"}); err != nil {
					t.Fatal(err)
				}
				var r Result
				if err := readJSON(filepath.Join(dir, "result.json"), &r); err != nil || State(&r, false) != "failed" || !r.Stopped {
					t.Fatalf("fallback result: %+v %v", r, err)
				}
			} else {
				var r Result
				if err := readJSON(filepath.Join(dir, "result.json"), &r); err != nil || State(&r, false) != "failed" || r.Signal != sig.String() {
					t.Fatalf("%+v %v", r, err)
				}
			}
			// Poll process exit rather than assuming signal delivery in 20ms.
			deadline = time.Now().Add(10 * time.Second)
			for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
				b, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
				if strings.Contains(string(b), ") Z ") {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if syscall.Kill(pid, 0) == nil {
				b, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
				if !strings.Contains(string(b), ") Z ") {
					t.Fatalf("orphan descendant %d", pid)
				}
			}
		})
	}
}

func TestReview1ValidatorConfigFailureStillCallsParent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"codex", "gh"} {
		body := "#!/bin/sh\nexit 0\n"
		if name == "gh" {
			body = "#!/bin/sh\nexit 1\n"
		}
		_ = os.WriteFile(filepath.Join(bin, name), []byte(body), 0700)
	}
	parent := filepath.Join(Root(), "example")
	child := filepath.Join(Root(), "example-v1")
	_ = os.MkdirAll(filepath.Join(child, "wt"), 0700)
	_ = os.MkdirAll(parent, 0700)
	_ = WriteJSON(filepath.Join(parent, "meta.json"), Meta{Order: Order{Name: "example"}})
	_ = WriteJSON(filepath.Join(parent, "result.json"), Result{Head: "sha", End: time.Now()})
	_ = WriteJSON(filepath.Join(child, "meta.json"), Meta{LaunchResolved: true, Parent: "example", ParentHost: Host{Local: true}, Revision: "sha", ConfigPath: filepath.Join(bin, "missing"), Order: Order{Name: "example-v1", Harness: "codex", Timeout: 1}})
	if err := Supervise("example-v1"); err != nil {
		t.Fatal(err)
	}
	records := validationRecords(parent)
	if len(records) != 1 || records[0].State != "fail" || !strings.Contains(records[0].Error, "validator config") {
		t.Fatalf("%+v", records)
	}
}
