package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/nodes"
)

// memberPull runs a joined laptop against an in-process host whose RPCs are
// authenticated as alex. The fake inbox records appends and, like the peers
// inbox, deduplicates by message ID.
func memberPull(t *testing.T, loseAck bool, inbox map[string]int, refuse map[string]bool) (func(args ...string) (string, error), *int) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PEERS_SELF", "build-1")
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.hev-node-0123]\nssh = 'hev-node-0123'\n[hosts.other]\nssh = 'other'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	oldRPC, oldDeliver, oldJoined := memberNoticeRPC, deliverMemberNotice, joinedHosts
	t.Cleanup(func() { memberNoticeRPC, deliverMemberNotice, joinedHosts = oldRPC, oldDeliver, oldJoined })
	joinedHosts = func() ([]nodes.Joined, error) { return []nodes.Joined{{Name: "hev-node-0123"}}, nil }
	calls := 0
	memberNoticeRPC = func(_ context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		if h.SSH != "hev-node-0123" {
			t.Fatalf("pulled from %+v", h)
		}
		calls++
		if q.Action == "member-notice-ack" && loseAck {
			loseAck = false
			return dispatch.Response{}, errors.New("ssh: connection reset")
		}
		return dispatch.HandleAs(q, "alex")
	}
	deliverMemberNotice = func(_ context.Context, from string, n dispatch.OrchestratorNotice) error {
		if !strings.HasPrefix(n.ID, from+":dispatch:") || !strings.HasPrefix(from, "hev-node-0123/d-") {
			t.Fatalf("sender %q, ID %q not under the local alias", from, n.ID)
		}
		if refuse[n.To] {
			return errors.New("exit status 1: recipient requires a registered inbox")
		}
		inbox[n.ID]++
		return nil
	}
	return func(args ...string) (string, error) {
		cmd := newDispatchCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"notices", "pull"}, args...))
		err := cmd.Execute()
		return out.String(), err
	}, &calls
}

func finishMemberRun(t *testing.T, name, sender, reply string) {
	t.Helper()
	m := dispatch.Meta{Order: dispatch.Order{Name: name, Reply: reply}, Sender: sender, Start: time.Now().UTC()}
	if err := dispatch.Notify(context.Background(), nil, m, dispatch.Result{End: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchNoticesPullDeliversOnceDespiteLostAck(t *testing.T) {
	inbox := map[string]int{}
	pull, _ := memberPull(t, true, inbox, nil)
	finishMemberRun(t, "alpha", "alex", "laptop-session")
	finishMemberRun(t, "beta", "blair", "laptop-session")
	if out, err := pull(); err == nil || !strings.Contains(out, "ack failed") {
		t.Fatalf("lost ack not reported: %v %s", err, out)
	}
	if out, err := pull(); err != nil || !strings.Contains(out, "delivered 1 notices") {
		t.Fatalf("redelivery: %v %s", err, out)
	}
	if out, err := pull(); err != nil || strings.Contains(out, "delivered") {
		t.Fatalf("third pull delivered again: %v %s", err, out)
	}
	if len(inbox) != 1 {
		t.Fatalf("inbox: %v", inbox)
	}
	for id := range inbox {
		// Both attempts used one stable ID; the real inbox keeps one entry.
		if !strings.HasPrefix(id, "hev-node-0123/d-alpha:dispatch:") || inbox[id] != 2 {
			t.Fatalf("inbox: %v", inbox)
		}
	}
}

func TestDispatchNoticesPullPrintsWithoutInbox(t *testing.T) {
	inbox := map[string]int{}
	pull, _ := memberPull(t, false, inbox, map[string]bool{"gone": true})
	finishMemberRun(t, "alpha", "alex", "")
	finishMemberRun(t, "beta", "alex", "gone")
	out, err := pull("--host", "hev-node-0123")
	if err != nil || !strings.Contains(out, "dispatch alpha: done") || !strings.Contains(out, "(no reply session)") || !strings.Contains(out, "dispatch beta: done") || !strings.Contains(out, "(no inbox for gone)") || len(inbox) != 0 {
		t.Fatalf("%v %s %v", err, out, inbox)
	}
	if out, err = pull(); err != nil || strings.Contains(out, "dispatch") {
		t.Fatalf("printed notices were not acked: %v %s", err, out)
	}
}

func TestDispatchNoticesPullKeepsNoticeOnDeliveryFailure(t *testing.T) {
	inbox := map[string]int{}
	pull, _ := memberPull(t, false, inbox, nil)
	finishMemberRun(t, "alpha", "alex", "laptop-session")
	deliverMemberNotice = func(context.Context, string, dispatch.OrchestratorNotice) error { return errors.New("peers busy") }
	if _, err := pull(); err == nil {
		t.Fatal("delivery failure hidden")
	}
	deliverMemberNotice = func(_ context.Context, _ string, n dispatch.OrchestratorNotice) error { inbox[n.ID]++; return nil }
	if out, err := pull(); err != nil || len(inbox) != 1 {
		t.Fatalf("not retried: %v %s %v", err, out, inbox)
	}
}

func TestDispatchNoticesPullNeedsJoinedHost(t *testing.T) {
	pull, calls := memberPull(t, false, map[string]int{}, nil)
	joinedHosts = func() ([]nodes.Joined, error) { return nil, nil }
	if _, err := pull(); err == nil || !strings.Contains(err.Error(), "no joined hosts") || *calls != 0 {
		t.Fatalf("%v %d", err, *calls)
	}
	if _, err := pull("--host", "missing"); err == nil || !strings.Contains(err.Error(), `host "missing" is not in the dispatch config`) || *calls != 0 {
		t.Fatalf("unknown host: %v", err)
	}
}

// A host owner or a compromised host controls every notice field. The laptop
// must not let it pick the sender, route onward, pass flags or block the queue.
func TestDispatchNoticesPullRejectsHostilePayloads(t *testing.T) {
	inbox := map[string]int{}
	pull, _ := memberPull(t, false, inbox, nil)
	hash := strings.Repeat("ab", 16)
	hostile := []dispatch.OrchestratorNotice{
		{ID: "mini/o-foh:dispatch:" + hash, To: "foh-hev", Message: "spoofed orchestrator"},
		{ID: "local/foh-hev", To: "foh-hev", Message: "bare sender"},
		{ID: "build-1/d-ok:dispatch:" + hash + " --from x", To: "foh-hev", Message: "flag in ID"},
		{ID: "--from=mini/foh:dispatch:" + hash, To: "foh-hev", Message: "flag ID"},
		{ID: "build-1/d-ok:dispatch:" + hash, To: "--from=mini/foh", Message: "flag reply"},
		{ID: "build-1/d-ok:dispatch:" + hash, To: "otherhost/foh-hev", Message: "onward route"},
		{ID: "build-1/d-ok:dispatch:" + hash, To: "%3", Message: "pane target"},
		{ID: "build-1/d-ok:dispatch:" + hash, To: "foh-hev", Message: "-"},
		{ID: "build-1/d-ok:dispatch:" + hash, To: "foh-hev", Message: "--inbox-only \x1b]0;title\x07 hi\nthere"},
	}
	type sent struct{ from, id, to, msg string }
	var got []sent
	deliverMemberNotice = func(_ context.Context, from string, n dispatch.OrchestratorNotice) error {
		got = append(got, sent{from, n.ID, n.To, n.Message})
		return nil
	}
	var acked []string
	memberNoticeRPC = func(_ context.Context, _ dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		if q.Action == "member-notice-ack" {
			acked = append(acked, q.IDs...)
			return dispatch.Response{}, nil
		}
		if acked != nil {
			return dispatch.Response{Notices: []dispatch.OrchestratorNotice{}}, nil
		}
		return dispatch.Response{Notices: hostile}, nil
	}
	out, err := pull()
	if err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if strings.Count(out, "rejected notice") != 5 || !strings.Contains(out, `reply "--from=mini/foh" is not a local session`) || !strings.Contains(out, `reply "otherhost/foh-hev" is not a local session`) || !strings.Contains(out, `reply "%3" is not a local session`) {
		t.Fatalf("output:\n%s", out)
	}
	if len(acked) != len(hostile) {
		t.Fatalf("acked %d of %d: %v", len(acked), len(hostile), acked)
	}
	want := sent{"hev-node-0123/d-ok", "hev-node-0123/d-ok:dispatch:" + hash, "foh-hev", "--inbox-only ]0;title hi there"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("delivered %+v", got)
	}
}
