//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bcc case: a run on a host that cannot reach the owner's laptop ends
// needs-decision; its push fails with an unresolvable hostname. Before the
// fix the notice survived only as notify_error, so the owner's pull got 0
// notices. It is now queued, pulled once and acknowledged.
func TestUnreachableOwnerRunNoticeIsPullable(t *testing.T) {
	dir, _, _ := outcomeFixture(t, "hung-ask")
	t.Setenv("FINAL_MARKER", "needs-decision hung: stop the run or wait for its timeout?")
	t.Setenv("PEERS_SELF", "win-oven")
	bin := filepath.SplitList(os.Getenv("PATH"))[0]
	if err := os.WriteFile(filepath.Join(bin, "unreachable"), []byte("#!/bin/sh\necho 'ssh: Could not resolve hostname macbook: Name or service not known' >&2\nexit 255\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		t.Fatal(err)
	}
	m.Reply = "Adams-MacBook-Air/foh-bcc"
	m.Sender = "local:hev"
	if err := os.WriteFile(m.ConfigPath, []byte(`notify = ["unreachable", "{reply}", "dispatch {name}: {state} {summary}"]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}

	if err := superviseRound("hung-ask"); err != nil {
		t.Fatal(err)
	}
	r := readResult(t, dir)
	if !strings.Contains(r.NotifyError, "Could not resolve hostname macbook") || !strings.Contains(r.NotifyError, "queued for hev orchestrate notices pull") {
		t.Fatalf("notify error %q", r.NotifyError)
	}

	// The owner's pull (hev orchestrate notices pull) now sees the notice.
	resp, err := Handle(Request{Action: "orchestrator-notices"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Notices) != 1 {
		t.Fatalf("pull got %d notices while one was waiting: %+v", len(resp.Notices), resp.Notices)
	}
	n := resp.Notices[0]
	if n.To != "Adams-MacBook-Air/foh-bcc" || !strings.HasPrefix(n.ID, "win-oven/d-hung-ask:dispatch:") || n.Route != "pull" ||
		!strings.Contains(n.Message, "needs-decision hung: stop the run or wait for its timeout?") || n.Attention == nil || n.Attention.Event != "needs-you" {
		t.Fatalf("notice %+v attention %+v", n, n.Attention)
	}
	// A specific orchestrator's notices do not include run notices.
	if _, err := Handle(Request{Action: "orchestrator-notices", Name: "nope"}); err == nil {
		t.Fatal("unknown orchestrator listed")
	}

	if _, err := Handle(Request{Action: "orchestrator-notice-ack", IDs: []string{n.ID}}); err != nil {
		t.Fatal(err)
	}
	if resp, err = Handle(Request{Action: "orchestrator-notices"}); err != nil || len(resp.Notices) != 0 {
		t.Fatalf("after ack: %+v %v", resp.Notices, err)
	}
}

// A push the owner's inbox refused is not unreachability and is not queued:
// a pull would fail the same way.
func TestRefusedOwnerPushIsNotQueued(t *testing.T) {
	for _, c := range []struct {
		err  string
		want bool
	}{
		{"exit status 255: ssh: Could not resolve hostname adams-macbook-air: nodename nor servname provided", true},
		{"exit status 255: ssh: connect to host 100.66.207.41 port 22: Connection refused", true},
		{"exit status 1: SSH peers RPC: exit status 255; delivery outcome may be unknown", true},
		{"exit status 1: relay pending: queued for recipient beacon; notices pull can also deliver", false},
		{"exit status 1: relay pending: recipient snapshot stale or has no active inbox", true},
		{"exit status 1: foh-bcc requires a registered inbox", false},
	} {
		if got := ownerUnreachable(errString(c.err)); got != c.want {
			t.Errorf("ownerUnreachable(%q) = %v, want %v", c.err, got, c.want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
