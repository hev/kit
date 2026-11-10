package dispatch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pullOnlyHost writes a dispatch config with owner_route = "pull", the host
// setting for an orchestrator host that cannot reach its owners.
func pullOnlyHost(t *testing.T) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(cfg, []byte("owner_route = \"pull\"\n[hosts.worker]\nssh = 'worker'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
}

func noTransport(t *testing.T) {
	t.Helper()
	old := notificationExecutable
	notificationExecutable = func() (string, error) { t.Fatal("pull-only owner exec'd a transport"); return "", nil }
	t.Cleanup(func() { notificationExecutable = old })
}

func decisionDriver(text string) OrchestratorDriver {
	return OrchestratorDriver{
		Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil },
		Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
			if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte(text), 0600); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Waiting on the owner."), 0600)
		},
		Now: time.Now,
	}
}

func notifySpec(t *testing.T) OrchestratorSpec {
	t.Helper()
	s, err := ApplyOrchestrator(strings.Replace(orchestratorExample, "authority: [say,", "authority: [notify, say,", 1), false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// localPull pulls from this process's own store as if over SSH. The inbox
// counts appends per ID; like the peers inbox, it keeps one row per ID.
func localPull(inbox map[string]int) NoticePull {
	return NoticePull{
		RPC:          func(_ context.Context, _ Host, q Request) (Response, error) { return Handle(q) },
		DeliverOwner: func(_ context.Context, n OrchestratorNotice) error { inbox[n.ID]++; return nil },
		Out:          io.Discard,
	}
}

func kindStatus(t *testing.T, name, kind string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, n := range noticesOf(t, name, kind) {
		out[n.Status]++
	}
	return out
}

// An owner unreachable by push: owner_route = "pull" on the orchestrator's
// host queues every notice for the pull and never execs a transport.
func TestOwnerRoutePullNeverPushes(t *testing.T) {
	orchHome(t)
	pullOnlyHost(t)
	noTransport(t)
	orchestratorNoticeSender = sendOrchestratorNotice
	s := notifySpec(t)
	if OrchestratorNoticeRoute(s) != "pull" {
		t.Fatal(OrchestratorNoticeRoute(s))
	}
	st, _ := orchestratorState(s.Name)
	if err := RunOrchestratorRound(context.Background(), s, &st, decisionDriver("Decision needed: scope?")); err != nil {
		t.Fatal(err)
	}
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	for _, n := range noticesOf(t, s.Name, "") {
		if n.Status != "queued" || n.Attempts != 0 || n.Error != "" {
			t.Fatal("pull-only notice was pushed", n)
		}
	}
	// A spec's notify_route still wins over the host default.
	s.NotifyRoute = "push"
	if OrchestratorNoticeRoute(s) != "push" {
		t.Fatal("spec route ignored")
	}
}

func TestOwnerRouteConfigValidated(t *testing.T) {
	dir := t.TempDir()
	for body, ok := range map[string]bool{"owner_route = 'pull'\n": true, "owner_route = 'push'\n": true, "owner_route = 'carrier'\n": false} {
		path := filepath.Join(dir, "dispatch.toml")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); (err == nil) != ok {
			t.Fatal(body, err)
		}
		if _, err := loadTarget(path, ""); (err == nil) != ok {
			t.Fatal("target", body, err)
		}
	}
}

// A reply that was never delivered survives stop, pause/resume and reapply,
// and stays pullable until delivered and acked. Only the new lineage's
// lifecycle notice supersedes the old lifecycle notices.
func TestUndeliveredReplySurvivesStopResume(t *testing.T) {
	orchHome(t)
	pullOnlyHost(t)
	noTransport(t)
	s := notifySpec(t)
	st, _ := orchestratorState(s.Name)
	if err := RunOrchestratorRound(context.Background(), s, &st, decisionDriver("Decision needed: ship scope A or B? Blocked on scope.")); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), st); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"pause", "resume", "stop"} {
		if action == "resume" {
			// Resume relaunches; this test drives rounds itself.
			_ = os.Remove(filepath.Join(OrchestratorDir(s.Name), "pause"))
			continue
		}
		if err := OrchestratorControl(s.Name, action, ""); err != nil {
			t.Fatal(action, err)
		}
	}
	advanceNotices(time.Second)
	s = notifySpec(t) // A stopped orchestrator resumes by reapplying.
	if got := kindStatus(t, s.Name, "reply"); got["queued"] != 1 || len(got) != 1 {
		t.Fatal("reply not kept queued", got)
	}
	// started, paused and stopped from the first lineage are superseded;
	// the second "started/applied" is queued.
	if got := kindStatus(t, s.Name, "lifecycle"); got["stale"] != 3 || got["queued"] != 1 {
		t.Fatal("lifecycle", got, noticesOf(t, s.Name, "lifecycle"))
	}
	inbox := map[string]int{}
	n, err := localPull(inbox).Owner(context.Background(), "worker", Host{SSH: "worker"}, "")
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	pending, _ := PendingOrchestratorNotices("")
	if len(pending) != 0 {
		t.Fatal("delivered notices still pending", pending)
	}
	if got := kindStatus(t, s.Name, "reply"); got["delivered"] != 1 {
		t.Fatal(got)
	}
}

// A reply an earlier release made stale is still returned, marked stale,
// until it is delivered and acked.
func TestStaleReplyStillPulled(t *testing.T) {
	orchHome(t)
	pullOnlyHost(t)
	s := notifySpec(t)
	if err := queueOrchestratorNotice(s, "reply", "old", "owner", "Decision needed: lost"); err != nil {
		t.Fatal(err)
	}
	if err := queueOrchestratorNotice(s, "lifecycle", "old", "owner", "stopped"); err != nil {
		t.Fatal(err)
	}
	all, _ := loadOrchestratorNotices(s.Name)
	for _, f := range all {
		if _, _, err := updateNotice(s.Name, f.path, func(n *OrchestratorNotice) bool { n.Status = "stale"; return true }); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := PendingOrchestratorNotices(s.Name)
	if err != nil || len(pending) != 1 || pending[0].Kind != "reply" || pending[0].Status != "stale" {
		t.Fatal("stale reply not pulled", pending, err)
	}
	if err := AckOrchestratorNotices([]string{pending[0].ID}); err != nil {
		t.Fatal(err)
	}
	if got := kindStatus(t, s.Name, "reply"); got["delivered"] != 1 {
		t.Fatal(got)
	}
	if got := kindStatus(t, s.Name, "lifecycle"); got["stale"] != 2 || got["delivered"] != 0 {
		t.Fatal("stale lifecycle notice was revived", got)
	}
}

// Lifecycle notices are superseded only by a newer lifecycle notice; replies
// and intake notices never are.
func TestSupersedeOnlyOlderLifecycle(t *testing.T) {
	orchHome(t)
	s := notifySpec(t) // Queues "started/applied".
	advanceNotices(time.Minute)
	for _, kind := range []string{"lifecycle", "reply", "intake"} {
		if err := queueOrchestratorNotice(s, kind, "k", "owner", "old "+kind); err != nil {
			t.Fatal(err)
		}
	}
	advanceNotices(time.Minute)
	newer, err := queueNotice(s, "lifecycle", "newer", "owner", "newer lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	advanceNotices(time.Minute)
	if err := queueOrchestratorNotice(s, "lifecycle", "later", "owner", "later lifecycle"); err != nil {
		t.Fatal(err)
	}
	if err := supersedeLifecycleNotices(s.Name, newer); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"started/applied": "stale", "old lifecycle": "stale", "old reply": "queued", "old intake": "queued", "newer lifecycle": "queued", "later lifecycle": "queued"}
	got := map[string]string{}
	for _, n := range noticesOf(t, s.Name, "") {
		for text := range want {
			if strings.HasSuffix(n.Message, text) {
				got[text] = n.Status
			}
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatal(got)
	}
}

// Delivery comes before the ack; a lost ack redelivers the same ID, which
// the inbox deduplicates, and the next pull finds nothing.
func TestOwnerPullLostAckDeduplicated(t *testing.T) {
	orchHome(t)
	pullOnlyHost(t)
	notifySpec(t)
	inbox := map[string]int{}
	p := localPull(inbox)
	lose := true
	p.RPC = func(_ context.Context, _ Host, q Request) (Response, error) {
		if q.Action == "orchestrator-notice-ack" && lose {
			lose = false
			return Response{}, errors.New("ssh: connection reset")
		}
		return Handle(q)
	}
	for i, wantErr := range []bool{true, false, false} {
		_, err := p.Owner(context.Background(), "worker", Host{SSH: "worker"}, "")
		if (err != nil) != wantErr {
			t.Fatal(i, err)
		}
	}
	if len(inbox) != 1 {
		t.Fatal(inbox)
	}
	for _, n := range inbox {
		if n != 2 {
			t.Fatal("want exactly one redelivery", inbox)
		}
	}
}

// One recipient that cannot be delivered to does not hold another's notices.
func TestOwnerPullFailedRecipientDoesNotBlockOthers(t *testing.T) {
	orchHome(t)
	pullOnlyHost(t)
	s := notifySpec(t)
	if err := queueOrchestratorNotice(s, "lifecycle", "x", "next-owner", "for next owner"); err != nil {
		t.Fatal(err)
	}
	inbox := map[string]int{}
	p := localPull(inbox)
	p.DeliverOwner = func(_ context.Context, n OrchestratorNotice) error {
		if n.To == "owner" {
			return errors.New("recipient requires a registered inbox")
		}
		inbox[n.ID]++
		return nil
	}
	n, err := p.Owner(context.Background(), "worker", Host{SSH: "worker"}, "")
	if err == nil || n != 1 || len(inbox) != 1 {
		t.Fatal(n, err, inbox)
	}
	pending, _ := PendingOrchestratorNotices("")
	if len(pending) != 1 || pending[0].To != "owner" {
		t.Fatal(pending)
	}
}

// A hung host costs only its own timeout; the others are delivered.
func TestNoticeSourcesHungHostDoesNotBlockOthers(t *testing.T) {
	orchHome(t)
	pullOnlyHost(t)
	notifySpec(t)
	inbox := map[string]int{}
	p := localPull(inbox)
	p.RPC = func(ctx context.Context, h Host, q Request) (Response, error) {
		if h.SSH == "hung" {
			<-ctx.Done()
			return Response{}, ctx.Err()
		}
		return Handle(q)
	}
	cfg := Config{Hosts: map[string]Host{"a-hung": {SSH: "hung"}, "b-good": {SSH: "good"}, "here": {Local: true}}}
	sources := NoticeSources(cfg, nil)
	if len(sources) != 2 || sources[0].Name != "a-hung" || sources[1].Member {
		t.Fatal("local host polled or wrong sources", sources)
	}
	start := time.Now()
	errs := p.Sources(context.Background(), sources, 300*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatal("hung host stalled the poll", elapsed)
	}
	if !errors.Is(errs["a-hung"], context.DeadlineExceeded) || errs["b-good"] != nil || len(inbox) != 1 {
		t.Fatal(errs, inbox)
	}
}

func TestNoticeSourcesJoinedHostsPullMemberNotices(t *testing.T) {
	cfg := Config{Hosts: map[string]Host{"node": {SSH: "node"}, "owned": {SSH: "owned"}}}
	got := NoticeSources(cfg, []string{"node", "not-configured"})
	if len(got) != 2 || !got[0].Member || got[0].Name != "node" || got[1].Member {
		t.Fatal(got)
	}
}

// ls reports an orchestrator whose decision request went unanswered for over
// 30 minutes as blocked:decision, with its undelivered notice backlog.
func TestBlockedOnDecisionState(t *testing.T) {
	orchHome(t)
	pullOnlyHost(t)
	s := notifySpec(t)
	st, _ := orchestratorState(s.Name)
	if err := RunOrchestratorRound(context.Background(), s, &st, decisionDriver("Decision needed: scope A or B?")); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), st); err != nil {
		t.Fatal(err)
	}
	state := func() OrchestratorState {
		rows, err := ListOrchestrators()
		if err != nil || len(rows) != 1 {
			t.Fatal(rows, err)
		}
		return rows[0]
	}
	if r := state(); r.State != "waiting" || r.Backlog == nil || r.Backlog.Count != 2 {
		t.Fatal("fresh decision", r.State, r.Backlog)
	}
	advanceNotices(31 * time.Minute)
	if r := state(); r.State != "blocked:decision" {
		t.Fatal(r.State)
	}
	if b := HostBacklog(); b.Count != 2 || b.Oldest.IsZero() {
		t.Fatal(b)
	}
	r, err := Handle(Request{Action: "list"})
	if err != nil || r.Backlog == nil || r.Backlog.Count != 2 {
		t.Fatal("list backlog", r.Backlog, err)
	}
	// An answer waiting in the inbox means the owner has replied.
	if err := SayOrchestrator(s.Name, "Scope A."); err != nil {
		t.Fatal(err)
	}
	if r := state(); r.State != "waiting" {
		t.Fatal("answered decision still blocked", r.State)
	}
}

func TestOrdinaryReplyIsNotBlocked(t *testing.T) {
	orchHome(t)
	s := notifySpec(t)
	st, _ := orchestratorState(s.Name)
	if err := RunOrchestratorRound(context.Background(), s, &st, decisionDriver("Ready to merge: head abc123.")); err != nil {
		t.Fatal(err)
	}
	_ = WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), st)
	advanceNotices(time.Hour)
	if rows, _ := ListOrchestrators(); rows[0].State != "waiting" {
		t.Fatal(rows[0].State)
	}
}
