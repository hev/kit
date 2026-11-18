package dispatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	ownerA = "desk/foh-a"
	ownerB = "desk/foh-b"
)

// adoptSpec is a loop orchestrator with notify authority owned by A.
func adoptSpec(extra string) string {
	text := strings.Replace(orchestratorExample, "reply: owner", "reply: "+ownerA+extra, 1)
	text = strings.Replace(text, "idle: skip", "idle: run", 1)
	return strings.Replace(text, "authority: [say, rework, retry, validate, dispatch]", "authority: [say, rework, retry, validate, dispatch, notify]", 1)
}

// noPane models the owner session that was cleared: every send to A fails.
func noPane(to string) error {
	return fmt.Errorf("peers: no pane on desk matches '%s'", strings.TrimPrefix(to, "desk/"))
}

type sentLog struct {
	mu   sync.Mutex
	sent []OrchestratorNotice
}

func (l *sentLog) send(_ context.Context, n OrchestratorNotice) error {
	if n.To == ownerA {
		return noPane(n.To)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, n)
	return nil
}

func (l *sentLog) all() []OrchestratorNotice {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]OrchestratorNotice{}, l.sent...)
}

func terminalState(t *testing.T, name string) {
	t.Helper()
	st, err := orchestratorState(name)
	if err != nil {
		t.Fatal(err)
	}
	if st.State == "complete" || st.State == "handed-off" || exists(filepath.Join(OrchestratorDir(name), "stop")) {
		t.Fatalf("orchestrator became terminal: %+v", st)
	}
}

// Acceptance: an orchestrator owned by A is adopted to B while its supervisor
// runs. The next round's notice goes to B, the round counter and checkpoint
// are unchanged by the adopt, and the state is never terminal.
func TestAdoptRunningOrchestratorNextNoticeGoesToNewOwner(t *testing.T) {
	orchHome(t)
	s, err := ApplyOrchestrator(adoptSpec(""), false)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	log := &sentLog{}
	calls, waits := 0, 0
	var roundsBefore int
	var checkpointBefore []byte
	d := OrchestratorDriver{
		Now:    func() time.Time { return clock },
		Notify: log.send,
		Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil },
		Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
			calls++
			if err := os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte(fmt.Sprintf("round %d checkpoint\n", calls)), 0600); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "reply.md"), []byte(fmt.Sprintf("round %d update", calls)), 0600)
		},
		Wait: func(context.Context) error {
			waits++
			clock = clock.Add(time.Second)
			terminalState(t, s.Name)
			dir := OrchestratorDir(s.Name)
			switch waits {
			case 1:
				if calls != 1 {
					t.Fatalf("calls %d before adopt", calls)
				}
				st, _ := orchestratorState(s.Name)
				roundsBefore = st.Rounds
				checkpointBefore, _ = os.ReadFile(filepath.Join(dir, "checkpoint.md"))
				if !processRecordLive(filepath.Join(dir, "supervisor.json")) {
					t.Fatal("supervisor is not live during adopt")
				}
				res, err := AdoptOrchestrator(s.Name, ownerB)
				if err != nil {
					t.Fatal(err)
				}
				if res.From != ownerA || res.To != ownerB || !res.Live {
					t.Fatalf("%+v", res)
				}
				st, _ = orchestratorState(s.Name)
				checkpoint, _ := os.ReadFile(filepath.Join(dir, "checkpoint.md"))
				if st.Rounds != roundsBefore || string(checkpoint) != string(checkpointBefore) {
					t.Fatalf("adopt touched rounds or checkpoint: %d/%d %q", st.Rounds, roundsBefore, checkpoint)
				}
			case 2:
				// The supervisor woke for the adopt without running a round.
				if calls != 1 {
					t.Fatalf("adopt started a round: calls %d", calls)
				}
				st, _ := orchestratorState(s.Name)
				if st.Rounds != roundsBefore || st.Reply != ownerB {
					t.Fatalf("after adopt %+v", st)
				}
				return os.WriteFile(filepath.Join(dir, "wake"), []byte("next"), 0600)
			case 3:
				return OrchestratorControl(s.Name, "stop", "")
			case 6:
				t.Fatal("supervisor did not stop")
			}
			return nil
		},
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	if calls != 2 || st.Rounds != roundsBefore+1 {
		t.Fatalf("calls %d rounds %d (before %d)", calls, st.Rounds, roundsBefore)
	}
	var round2, adopted, applied bool
	for _, n := range log.all() {
		if n.To != ownerB {
			t.Fatalf("notice to %s after adopt: %+v", n.To, n)
		}
		round2 = round2 || n.Message == "round 2 update"
		adopted = adopted || strings.Contains(n.Message, "adopted: owner "+ownerA+" -> "+ownerB)
		applied = applied || strings.Contains(n.Message, "started/applied")
	}
	if !round2 || !adopted || !applied {
		t.Fatalf("round2=%v adopted=%v applied(re-delivered)=%v: %+v", round2, adopted, applied, log.all())
	}
	spec, _ := LoadOrchestrator(s.Name)
	stored, _ := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "spec.md"))
	if spec.Reply != ownerB || !strings.Contains(string(stored), "reply: "+ownerB) || !strings.Contains(string(stored), "Keep moving.") {
		t.Fatalf("stored spec: %s", stored)
	}
}

// Acceptance: notices queued and failed for A are re-delivered to B; already
// delivered ones are not sent again.
func TestAdoptRedeliversQueuedAndFailedNotices(t *testing.T) {
	orchHome(t)
	log := &sentLog{}
	orchestratorNoticeSender = log.send
	s, err := ApplyOrchestrator(adoptSpec(""), false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	for _, k := range []string{"queued", "failed", "delivered"} {
		if err := queueOrchestratorNotice(s, "reply", k, ownerA, k+" notice"); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := loadOrchestratorNotices(s.Name)
	for _, f := range all {
		status := strings.TrimSuffix(f.n.Message, " notice")
		if status == "failed" || status == "delivered" {
			if _, _, err := updateNotice(s.Name, f.path, func(n *OrchestratorNotice) bool { n.Status = status; return true }); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A was cleared: the queued ones fail and back off.
	flushOrchestratorNotices(s, orchestratorNoticeSender)
	if len(log.all()) != 0 {
		t.Fatal(log.all())
	}
	res, err := AdoptOrchestrator(s.Name, ownerB)
	if err != nil {
		t.Fatal(err)
	}
	if res.Live || res.Redelivered != 3 { // applied, queued, failed
		t.Fatalf("%+v", res)
	}
	got := map[string]bool{}
	for _, n := range log.all() {
		if n.To != ownerB {
			t.Fatal(n)
		}
		got[n.Message] = true
	}
	if !got["queued notice"] || !got["failed notice"] || got["delivered notice"] || len(log.all()) != 4 {
		t.Fatalf("%+v", log.all())
	}
	// Re-running the move is a no-op.
	if n, err := retargetOrchestratorNotices(s.Name, ownerA, ownerB); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if after, _ := orchestratorState(s.Name); after.Rounds != st.Rounds || after.State != "ready" {
		t.Fatalf("%+v", after)
	}
}

// Acceptance: with notify_route: pull, re-delivered notices stay pullable,
// now addressed to B, and nothing is pushed.
func TestAdoptPullRouteNoticesPullableByNewOwner(t *testing.T) {
	orchHome(t)
	pushed := 0
	orchestratorNoticeSender = func(context.Context, OrchestratorNotice) error { pushed++; return nil }
	s, err := ApplyOrchestrator(adoptSpec("\nnotify_route: pull"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := queueOrchestratorNotice(s, "reply", "k", ownerA, "decision needed"); err != nil {
		t.Fatal(err)
	}
	if _, err := AdoptOrchestrator(s.Name, ownerB); err != nil {
		t.Fatal(err)
	}
	pending, err := PendingOrchestratorNotices(s.Name)
	if err != nil {
		t.Fatal(err)
	}
	var decision bool
	for _, n := range pending {
		if n.To != ownerB {
			t.Fatalf("pending notice for %s: %+v", n.To, n)
		}
		decision = decision || n.Message == "decision needed"
	}
	if !decision || pushed != 0 {
		t.Fatalf("pushed %d pending %+v", pushed, pending)
	}
	var ids []string
	for _, n := range pending {
		ids = append(ids, n.ID)
	}
	if err := AckOrchestratorNotices(ids); err != nil {
		t.Fatal(err)
	}
	if pending, _ = PendingOrchestratorNotices(s.Name); len(pending) != 0 {
		t.Fatal(pending)
	}
}

func TestAdoptRefusesTerminalAndNamesApply(t *testing.T) {
	orchHome(t)
	s, err := ApplyOrchestrator(adoptSpec(""), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	_, err = AdoptOrchestrator(s.Name, ownerB)
	if err == nil || !strings.Contains(err.Error(), "hev orchestrate apply") || !strings.Contains(err.Error(), "terminal") {
		t.Fatal(err)
	}
	if spec, _ := LoadOrchestrator(s.Name); spec.Reply != ownerA {
		t.Fatal("refused adopt changed the spec")
	}
	if _, err = AdoptOrchestrator(s.Name, "bad owner"); err == nil {
		t.Fatal("invalid owner accepted")
	}
}

func TestSetSpecReplyAddsAndReplaces(t *testing.T) {
	for _, text := range []string{orchestratorExample, strings.Replace(orchestratorExample, "reply: owner\n", "", 1)} {
		out, err := setSpecReply(text, ownerB)
		if err != nil {
			t.Fatal(err)
		}
		s, err := ParseOrchestrator(out)
		if err != nil || s.Reply != ownerB || s.Brief != "Keep moving." || s.Budget.RunsPerDay != 20 {
			t.Fatalf("%+v %v\n%s", s, err, out)
		}
	}
}
