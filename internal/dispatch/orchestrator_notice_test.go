package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func noticeDriver(t *testing.T, send OrchestratorNotifier) OrchestratorDriver {
	t.Helper()
	return OrchestratorDriver{
		Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil },
		Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
			if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte("Ready to merge: head abc123; evidence: checks passed."), 0600); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Goal pending. Reply notes: ready to merge."), 0600)
		},
		Now: time.Now, Notify: send,
	}
}
func TestOrchestratorReplyAcceptedOnceAndGated(t *testing.T) {
	for _, mode := range []string{"accepted", "no-authority", "no-reply", "failed", "empty-checkpoint", "silent"} {
		t.Run(mode, func(t *testing.T) {
			orchHome(t)
			s := applyOrch(t)
			s.Authority = append(s.Authority, "notify")
			if mode == "no-authority" {
				s.Authority = []string{"say"}
			}
			if mode == "no-reply" {
				s.Reply = ""
			}
			calls := 0
			d := noticeDriver(t, func(_ context.Context, n OrchestratorNotice) error {
				calls++
				if n.To != "owner" {
					t.Fatal(n)
				}
				return nil
			})
			session := d.Session
			d.Session = func(ctx context.Context, s OrchestratorSpec, dir string, p OrchestratorPacket) error {
				if err := session(ctx, s, dir, p); err != nil {
					return err
				}
				switch mode {
				case "failed":
					return errors.New("harness failed")
				case "empty-checkpoint":
					return os.WriteFile(filepath.Join(dir, "checkpoint.md"), nil, 0600)
				case "silent":
					return os.Remove(filepath.Join(dir, "reply.md"))
				}
				return nil
			}
			st, _ := orchestratorState(s.Name)
			err := RunOrchestratorRound(context.Background(), s, &st, d)
			failed := mode == "failed" || mode == "empty-checkpoint"
			if (err != nil) != failed {
				t.Fatal(err)
			}
			want := 0
			if mode == "accepted" {
				want = 1
			}
			if calls != want {
				t.Fatalf("calls=%d want=%d", calls, want)
			}
			if !failed {
				if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
					t.Fatal(err)
				}
				if st.Rounds != 1 || calls != want {
					t.Fatal("idle sent a new notice", st, calls)
				}
			}
			retryOrchestratorNotices(s.Name, d.Notify)
			if calls != want {
				t.Fatal("duplicate", calls)
			}
		})
	}
}
func TestOrchestratorNoticeQueuedRetryAndUncertainSend(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	s.Authority = append(s.Authority, "notify")
	calls := 0
	received := map[string]bool{}
	delivered := 0
	d := noticeDriver(t, func(_ context.Context, n OrchestratorNotice) error {
		calls++
		if calls == 1 {
			return errors.New("owner unreachable")
		}
		if !received[n.ID] {
			received[n.ID] = true
			delivered++
		}
		if calls == 2 {
			return errors.New("receipt lost after enqueue")
		}
		return nil
	})
	st, _ := orchestratorState(s.Name)
	if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
		t.Fatal("delivery failed round", err)
	}
	read := func() OrchestratorNotice {
		var n OrchestratorNotice
		if err := readJSON(filepath.Join(OrchestratorDir(s.Name), "rounds", "001", "notice.json"), &n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := read(); n.Status != "queued" || n.Error == "" {
		t.Fatal(n)
	}
	// Later wakes retry even if evidence is unchanged. The recipient deduplicates
	// an uncertain successful enqueue using the same immutable ID and payload.
	for i := 0; i < 3; i++ {
		if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 || delivered != 1 || read().Status != "delivered" || read().Error != "" {
		t.Fatal(calls, delivered, read())
	}
	rows, err := ListOrchestrators()
	if err != nil || len(rows) != 1 || rows[0].LastNotice == nil || rows[0].LastNotice.Status != "delivered" {
		t.Fatal(rows, err)
	}
}
func TestOrchestratorLifecycleNotices(t *testing.T) {
	orchHome(t)
	var notices []OrchestratorNotice
	orchestratorNoticeSender = func(_ context.Context, n OrchestratorNotice) error { notices = append(notices, n); return nil }
	s := applyOrch(t)
	check := func(event string) {
		t.Helper()
		for _, n := range notices {
			if strings.Contains(n.Message, event) {
				if !strings.Contains(n.Message, s.Name) || !strings.Contains(n.Message, " on ") {
					t.Fatal(n)
				}
				return
			}
		}
		t.Fatal("missing notice", event, notices)
	}
	check("started/applied")
	if err := OrchestratorControl(s.Name, "pause", ""); err != nil {
		t.Fatal(err)
	}
	check("paused")
	// A recorded supervisor keeps resume local; no real harness or tmux needed.
	if err := writePID(OrchestratorDir(s.Name)); err != nil {
		t.Fatal(err)
	}
	if err := OrchestratorControl(s.Name, "resume", ""); err != nil {
		t.Fatal(err)
	}
	check("resumed")
	_ = os.Remove(filepath.Join(OrchestratorDir(s.Name), "supervisor.json"))
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	check("stopped")
	notices = nil
	s = applyOrch(t)
	st, _ := orchestratorState(s.Name)
	orchestratorBudgetNotice(s, "runs_per_day")
	check("budget exhausted")
	notices = nil
	if err := OrchestratorControl(s.Name, "handoff", "next-owner"); err != nil {
		t.Fatal(err)
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, noticeDriver(t, orchestratorNoticeSender)); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 2 || notices[0].To == notices[1].To {
		t.Fatal("handoff needs both owners", notices)
	}
	check("handoff to next-owner")
	notices = nil
	s = applyOrch(t)
	s.Mode = "once"
	d := noticeDriver(t, orchestratorNoticeSender)
	// Persist the once mode and drive an accepted round to completion.
	raw, _ := os.ReadFile(filepath.Join(OrchestratorConfigDir(), s.Name+".md"))
	raw = []byte(strings.Replace(string(raw), "mode: loop", "mode: once", 1))
	if _, err := ApplyOrchestrator(string(raw), false); err != nil {
		t.Fatal(err)
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	check("goal met (complete)")
	// Two consecutive failures emit one code-authored warning, and a success
	// resets the streak. No sleep, timeout, network or model needed.
	notices = nil
	s = applyOrch(t)
	st, _ = orchestratorState(s.Name)
	calls := 0
	clock := st.Start
	d = noticeDriver(t, orchestratorNoticeSender)
	d.Now = func() time.Time { return clock }
	d.Session = func(context.Context, OrchestratorSpec, string, OrchestratorPacket) error {
		calls++
		return context.DeadlineExceeded
	}
	d.Wait = func(context.Context) error {
		clock = clock.Add(20 * time.Minute)
		if calls == 3 {
			return os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "stop"), nil, 0600)
		}
		return nil
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, n := range notices {
		if strings.Contains(n.Message, "2 consecutive failures") {
			count++
		}
	}
	if count != 1 {
		t.Fatal(notices)
	}
}
func TestOrchestratorReplyTruncated(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	s.Authority = []string{"notify"}
	st, _ := orchestratorState(s.Name)
	d := noticeDriver(t, func(_ context.Context, n OrchestratorNotice) error {
		if len([]rune(n.Message)) > 2000 || !strings.Contains(n.Message, "hev orchestrate show "+s.Name) {
			t.Fatal(n)
		}
		return nil
	})
	original := d.Session
	d.Session = func(ctx context.Context, s OrchestratorSpec, dir string, p OrchestratorPacket) error {
		if err := original(ctx, s, dir, p); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "reply.md"), []byte(strings.Repeat("界", 3000)), 0600)
	}
	if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
		t.Fatal(err)
	}
}

func TestOrchestratorLifecycleNoReplyAndBudgetExpiry(t *testing.T) {
	orchHome(t)
	calls := []OrchestratorNotice{}
	orchestratorNoticeSender = func(_ context.Context, n OrchestratorNotice) error { calls = append(calls, n); return nil }
	raw := strings.Replace(orchestratorExample, "reply: owner\n", "", 1)
	s, err := ApplyOrchestrator(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	orchestratorLifecycle(s, st, "handoff", "handoff", orchestratorNoticeSender, "next-owner")
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatal("lifecycle without reply", calls)
	}
	s = applyOrch(t)
	st, _ = orchestratorState(s.Name)
	calls = nil
	d := noticeDriver(t, orchestratorNoticeSender)
	d.Now = func() time.Time { return st.Start.Add(8 * 24 * time.Hour) }
	d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		t.Fatal("expired lifetime gathered")
		return nil, nil
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !strings.Contains(calls[0].Message, "budget exhausted") {
		t.Fatal(calls)
	}
	calls = nil
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatal("terminal duplicate", calls)
	}
}
