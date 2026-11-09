package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

const pollSpec = "---\nkind: orchestrator\nname: scoped\ngoal: Ship\nreply: owner\nauthority: [notify]\nmanages: {runs: {names: [task]}}\n---\nBrief\n"

// pollHome is an orchestrator host whose owner is unreachable by push: its
// dispatch config sets owner_route = "pull".
func pollHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORCHESTRATOR", "")
	t.Setenv("PEERS_SELF", "oven")
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(cfg, []byte("owner_route = \"pull\"\n[hosts.oven]\nssh = 'oven'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	if _, err := dispatch.ApplyOrchestrator(pollSpec, false); err != nil {
		t.Fatal(err)
	}
}

// replyRound runs one round that asks the owner a question.
func replyRound(t *testing.T, text string) {
	t.Helper()
	s, err := dispatch.LoadOrchestrator("scoped")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dispatch.ListOrchestrators()
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	st := rows[0]
	d := dispatch.OrchestratorDriver{
		Gather: func(context.Context, dispatch.OrchestratorSpec) ([]dispatch.OrchestratorEvidence, error) {
			return nil, nil
		},
		Session: func(_ context.Context, _ dispatch.OrchestratorSpec, dir string, _ dispatch.OrchestratorPacket) error {
			if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte(text), 0600); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte(text), 0600)
		},
		Now: time.Now,
		Notify: func(context.Context, dispatch.OrchestratorNotice) error {
			t.Fatal("pull-only owner was pushed to")
			return nil
		},
	}
	if err := dispatch.RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
		t.Fatal(err)
	}
}

// fakeInbox is the owner's peers inbox: one row per ID, counting appends.
type fakeInbox struct {
	mu       sync.Mutex
	appends  map[string]int
	messages map[string]string
}

func (f *fakeInbox) deliver(_ context.Context, n dispatch.OrchestratorNotice) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appends[n.ID]++
	f.messages[n.ID] = n.Message
	return nil
}

func (f *fakeInbox) has(substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

// testPoll pulls from the in-process host "oven" as if over SSH.
func testPoll(inbox *fakeInbox, rpc func(context.Context, dispatch.Host, dispatch.Request) (dispatch.Response, error)) *noticePoll {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := newNoticePoll(logger)
	p.pull.RPC = rpc
	p.pull.DeliverOwner = inbox.deliver
	p.pull.DeliverMember = func(context.Context, string, dispatch.OrchestratorNotice) error {
		return errors.New("no member notices expected")
	}
	p.load = func() (dispatch.Config, error) {
		return dispatch.Config{Hosts: map[string]dispatch.Host{"oven": {SSH: "oven"}}}, nil
	}
	p.joined = func() ([]string, error) { return nil, nil }
	return p
}

func handleOven(_ context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
	if h.SSH != "oven" {
		return dispatch.Response{}, errors.New("unexpected host " + h.SSH)
	}
	return dispatch.Handle(q)
}

// The kit-063 incident: a decision request queued for an owner the host
// cannot reach survives orchestrate stop and the reapply that resumes it,
// and hevd's poll delivers it within one interval.
func TestNoticePollDeliversReplyAfterStopResume(t *testing.T) {
	pollHome(t)
	replyRound(t, "Decision needed: ship scope A or B?")
	if err := dispatch.OrchestratorControl("scoped", "stop", ""); err != nil {
		t.Fatal(err)
	}
	// A stopped orchestrator resumes by reapplying.
	if _, err := dispatch.ApplyOrchestrator(pollSpec, false); err != nil {
		t.Fatal(err)
	}
	inbox := &fakeInbox{appends: map[string]int{}, messages: map[string]string{}}
	p := testPoll(inbox, handleOven)
	const interval = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); p.run(ctx, interval) }()
	t.Cleanup(func() { cancel(); <-done })

	within := func(what string, d time.Duration) {
		t.Helper()
		deadline := time.Now().Add(d)
		for !inbox.has(what) {
			if time.Now().After(deadline) {
				t.Fatalf("%q not delivered within %s", what, d)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	within("Decision needed: ship scope A or B?", interval)
	// A notice queued between polls arrives on the next tick.
	replyRound(t, "Decision needed: second question")
	within("Decision needed: second question", interval+250*time.Millisecond)
	waitPending(t, 0)
}

func waitPending(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		pending, err := dispatch.PendingOrchestratorNotices("")
		if err == nil && len(pending) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("pending", pending, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A lost ack makes the next poll deliver the same IDs again; the inbox keeps
// one row per ID, and the poll after that finds nothing.
func TestNoticePollLostAckDeduplicated(t *testing.T) {
	pollHome(t)
	replyRound(t, "Decision needed: scope?")
	inbox := &fakeInbox{appends: map[string]int{}, messages: map[string]string{}}
	lose := true
	p := testPoll(inbox, func(ctx context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		if q.Action == "orchestrator-notice-ack" && lose {
			lose = false
			return dispatch.Response{}, errors.New("ssh: connection reset")
		}
		return handleOven(ctx, h, q)
	})
	for range 3 {
		p.once(context.Background())
	}
	if len(inbox.appends) != 2 { // started/applied and the reply.
		t.Fatal(inbox.appends)
	}
	for id, n := range inbox.appends {
		if n != 2 {
			t.Fatal("want exactly one redelivery", id, n)
		}
	}
	waitPending(t, 0)
}

// One hung host costs only its timeout; the others are delivered.
func TestNoticePollHungHostDoesNotBlockOthers(t *testing.T) {
	pollHome(t)
	replyRound(t, "Decision needed: scope?")
	inbox := &fakeInbox{appends: map[string]int{}, messages: map[string]string{}}
	p := testPoll(inbox, func(ctx context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		if h.SSH == "hung" {
			<-ctx.Done()
			return dispatch.Response{}, ctx.Err()
		}
		return handleOven(ctx, h, q)
	})
	p.load = func() (dispatch.Config, error) {
		return dispatch.Config{Hosts: map[string]dispatch.Host{"hung": {SSH: "hung"}, "oven": {SSH: "oven"}}}, nil
	}
	p.timeout = 300 * time.Millisecond
	start := time.Now()
	p.once(context.Background())
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatal("hung host stalled the poll", elapsed)
	}
	if !inbox.has("Decision needed: scope?") {
		t.Fatal(inbox.messages)
	}
	if !strings.Contains(p.last["hung"], "deadline") || p.last["oven"] != "" {
		t.Fatal(p.last)
	}
}

func TestNoticePullIntervalConfig(t *testing.T) {
	for v, want := range map[string]time.Duration{"off": 0, "0": 0, "30s": 30 * time.Second, "2m": 2 * time.Minute} {
		if got, err := parseNoticePullInterval(v); err != nil || got != want {
			t.Fatal(v, got, err)
		}
	}
	for _, v := range []string{"10ms", "soon"} {
		if _, err := parseNoticePullInterval(v); err == nil {
			t.Fatal("accepted", v)
		}
	}
}
