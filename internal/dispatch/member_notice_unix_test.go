//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeliverMemberNoticeEndsFlagsBeforeHostText(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	exe := filepath.Join(dir, "hev")
	if e := os.WriteFile(exe, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \""+log+"\"\n"), 0700); e != nil {
		t.Fatal(e)
	}
	old := notificationExecutable
	notificationExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { notificationExecutable = old })
	hash := strings.Repeat("ab", 16)
	n, from, e := LocalMemberNotice("hev-node-0123", OrchestratorNotice{ID: "build/d-alpha:dispatch:" + hash, To: "laptop-session", Message: "--from mini/foh"})
	if e != nil {
		t.Fatal(e)
	}
	if e = DeliverMemberNotice(context.Background(), from, n); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(log)
	want := "peers\nsend\n--from\nhev-node-0123/d-alpha\n--id\nhev-node-0123/d-alpha:dispatch:" + hash + "\n--inbox-only\n--\nlaptop-session\n--from mini/foh\n"
	if string(b) != want {
		t.Fatalf("args:\n%s", b)
	}
}

// A store failure is logged; owner-side templates still run.
func TestMemberNoticeQueueFailureKeepsOwnerTemplates(t *testing.T) {
	memberNoticeTest(t)
	if e := os.MkdirAll(Root(), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(Root(), ".member-notices"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "owner")
	owner := []string{"/bin/sh", "-c", "printf '%s' \"$1\" > " + out, "sh", "{name} {state}"}
	end := Result{End: time.Date(2026, 11, 7, 11, 30, 0, 0, time.UTC)}
	if e := Notify(context.Background(), owner, memberMeta("alpha", "alex"), end); e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(out); string(b) != "alpha done" {
		t.Fatalf("owner template: %q", b)
	}
}
