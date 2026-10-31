//go:build !windows

package dispatch

import (
	"os"
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
	m := Meta{Order: Order{Name: "test-stop", Harness: "codex", Timeout: 1}, Branch: "d/test-stop", Notify: []string{"missing-notify-command"}}
	if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- Supervise("test-stop") }()
	deadline := time.Now().Add(5 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		b, e := os.ReadFile(filepath.Join(dir, "child.pid"))
		if e == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			break
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
	case <-time.After(5 * time.Second):
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
