package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// noticeLog records sends; safe for senders running in other goroutines.
type noticeLog struct {
	mu   sync.Mutex
	sent []OrchestratorNotice
	fail func(OrchestratorNotice) error
}

func (l *noticeLog) send(_ context.Context, n OrchestratorNotice) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, n)
	if l.fail != nil {
		return l.fail(n)
	}
	return nil
}
func (l *noticeLog) count(substr string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := 0
	for _, n := range l.sent {
		if strings.Contains(n.Message, substr) {
			c++
		}
	}
	return c
}
func (l *noticeLog) reset() { l.mu.Lock(); l.sent = nil; l.mu.Unlock() }

func noticesOf(t *testing.T, name, kind string) []OrchestratorNotice {
	t.Helper()
	all, err := loadOrchestratorNotices(name)
	if err != nil {
		t.Fatal(err)
	}
	var out []OrchestratorNotice
	for _, f := range all {
		if kind == "" || f.n.Kind == kind {
			out = append(out, f.n)
		}
	}
	return out
}

// advanceNotices moves the notice clock past any backoff.
func advanceNotices(by time.Duration) {
	now := noticeNow().Add(by)
	noticeNow = func() time.Time { return now }
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
			flushOrchestratorNotices(s, func(context.Context, OrchestratorNotice) error { return nil })
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
			flushOrchestratorNotices(s, d.Notify)
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
	flushOrchestratorNotices(s, func(context.Context, OrchestratorNotice) error { return nil })
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
		n := noticesOf(t, s.Name, "reply")
		if len(n) != 1 {
			t.Fatal(n)
		}
		return n[0]
	}
	if n := read(); n.Status != "queued" || n.Error == "" || n.Attempts != 1 {
		t.Fatal(n)
	}
	// Later wakes retry after the backoff even if evidence is unchanged. The
	// recipient deduplicates an uncertain enqueue by the immutable ID.
	for i := 0; i < 3; i++ {
		advanceNotices(2 * noticeMaxBackoff)
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
	log := &noticeLog{}
	orchestratorNoticeSender = log.send
	s := applyOrch(t)
	check := func(event string) {
		t.Helper()
		flushOrchestratorNotices(s, orchestratorNoticeSender)
		for _, n := range log.sent {
			if strings.Contains(n.Message, event) {
				if !strings.Contains(n.Message, s.Name) || !strings.Contains(n.Message, " on test-host") {
					t.Fatal(n)
				}
				return
			}
		}
		t.Fatal("missing notice", event, log.sent)
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
	log.reset()
	s = applyOrch(t)
	orchestratorBudgetNotice(s, "runs_per_day")
	check("budget exhausted")
	log.reset()
	if err := OrchestratorControl(s.Name, "handoff", "next-owner"); err != nil {
		t.Fatal(err)
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, noticeDriver(t, orchestratorNoticeSender)); err != nil {
		t.Fatal(err)
	}
	if len(log.sent) != 2 || log.sent[0].To == log.sent[1].To {
		t.Fatal("handoff needs both owners", log.sent)
	}
	check("handoff to next-owner")
	log.reset()
	// Persist the once mode and drive an accepted round to completion.
	raw, _ := os.ReadFile(filepath.Join(OrchestratorConfigDir(), s.Name+".md"))
	raw = []byte(strings.Replace(string(raw), "mode: loop", "mode: once", 1))
	if _, err := ApplyOrchestrator(string(raw), false); err != nil {
		t.Fatal(err)
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, noticeDriver(t, orchestratorNoticeSender)); err != nil {
		t.Fatal(err)
	}
	check("goal met (complete)")
	// Two consecutive failures emit one code-authored warning, and a success
	// resets the streak. No sleep, timeout, network or model needed.
	log.reset()
	s = applyOrch(t)
	st, _ := orchestratorState(s.Name)
	calls := 0
	clock := st.Start
	d := noticeDriver(t, orchestratorNoticeSender)
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
	if log.count("2 consecutive failures") != 1 {
		t.Fatal(log.sent)
	}
}

func TestOrchestratorReplyTruncated(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	s.Authority = []string{"notify"}
	st, _ := orchestratorState(s.Name)
	d := noticeDriver(t, func(_ context.Context, n OrchestratorNotice) error {
		if len([]rune(n.Message)) > 2000 || (n.Kind == "reply" && !strings.Contains(n.Message, "hev orchestrate show "+s.Name)) {
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
	log := &noticeLog{}
	orchestratorNoticeSender = log.send
	raw := strings.Replace(orchestratorExample, "reply: owner\n", "", 1)
	s, err := ApplyOrchestrator(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	_ = orchestratorLifecycle(s, st, "handoff", "handoff", "next-owner")
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	if len(log.sent) != 0 || len(noticesOf(t, s.Name, "")) != 0 {
		t.Fatal("lifecycle without reply", log.sent)
	}
	s = applyOrch(t)
	st, _ = orchestratorState(s.Name)
	d := noticeDriver(t, orchestratorNoticeSender)
	d.Now = func() time.Time { return st.Start.Add(8 * 24 * time.Hour) }
	d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		t.Fatal("expired lifetime gathered")
		return nil, nil
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	if log.count("budget exhausted") != 1 {
		t.Fatal(log.sent)
	}
	log.reset()
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	if len(log.sent) != 0 {
		t.Fatal("terminal duplicate", log.sent)
	}
}

// B1: no send happens under the dispatch root lock or the orchestrator lock,
// and apply, say and admission complete while a sender blocks forever.
func TestNoticeSendHoldsNoLock(t *testing.T) {
	orchHome(t)
	if err := os.MkdirAll(Root(), 0700); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var mu sync.Mutex
	var held []string
	blocked := make(chan OrchestratorNotice, 16)
	checkLocks := func(n OrchestratorNotice) {
		mu.Lock()
		defer mu.Unlock()
		if lockHeld(Root()) {
			held = append(held, "root:"+n.Message)
		}
		if lockHeld(OrchestratorDir("release-train")) {
			held = append(held, "orchestrator:"+n.Message)
		}
	}
	orchestratorNoticeSender = func(_ context.Context, n OrchestratorNotice) error {
		checkLocks(n)
		blocked <- n
		<-release // Blocks for the rest of the test, ignoring its deadline.
		return nil
	}
	within := func(what string, fn func() error) {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- fn() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(what, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal(what, "blocked on a notice send")
		}
	}
	var s OrchestratorSpec
	within("apply", func() (err error) { s, err = ApplyOrchestrator(orchestratorExample, false); return })
	s.Authority = append(s.Authority, "notify")
	within("say", func() error { return SayOrchestrator(s.Name, "operator note") })
	// A round consumes the inbox under the root lock and queues its reply; its
	// delivery blocks forever, after the lock is released.
	st, _ := orchestratorState(s.Name)
	roundDone := make(chan struct{})
	go func() {
		defer close(roundDone)
		_ = RunOrchestratorRound(context.Background(), s, &st, noticeDriver(t, orchestratorNoticeSender))
	}()
	// Join the round before TempDir cleanup: releasing its notifier alone leaves
	// receipt writes racing directory removal.
	defer func() { close(release); <-roundDone }()
	select {
	case <-blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("round never delivered")
	}
	within("say during blocked send", func() error { return SayOrchestrator(s.Name, "second note") })
	// Admission refuses an exhausted budget under the root and orchestrator
	// locks; the budget notice is only queued there.
	within("admission", func() error {
		unlockRoot, err := lockRoot()
		if err != nil {
			return err
		}
		defer unlockRoot()
		unlock, err := lockDirectory(OrchestratorDir(s.Name))
		if err != nil {
			return err
		}
		defer unlock()
		if err = reserveOrchestratorCounts(s, s.Budget.ConcurrentRuns, "budget.json"); err == nil {
			return errors.New("budget admitted")
		}
		return nil
	})
	if n := noticesOf(t, s.Name, "lifecycle"); len(n) < 2 || !strings.Contains(n[len(n)-1].Message, "budget exhausted") || n[len(n)-1].Status != "queued" {
		t.Fatal("budget notice not queued", n)
	}
	// After unlocking, delivery takes the queued receipt.
	orchestratorNoticeSender = func(_ context.Context, n OrchestratorNotice) error { checkLocks(n); return nil }
	advanceNotices(time.Hour)
	within("stop", func() error { return OrchestratorControl(s.Name, "stop", "") })
	for _, n := range noticesOf(t, s.Name, "lifecycle") {
		if strings.Contains(n.Message, "budget exhausted") && n.Status != "delivered" {
			t.Fatal("budget receipt not delivered after unlock", n)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(held) != 0 {
		t.Fatal("sent while holding a lock", held)
	}
}

// N1, F5: ten capped rounds defer, stay waiting, create no round directory,
// send no failure notice and yield one notice per cap episode.
func TestCapEpisodeOneNoticeTenRounds(t *testing.T) {
	s := sourceFixture(t)
	log := &noticeLog{}
	flushOrchestratorNotices(s, log.send)
	log.reset()
	st, _ := orchestratorState(s.Name)
	f := newFakeTarget(t)
	s.Budget.RunsPerDay = 1
	budget := filepath.Join(OrchestratorDir(s.Name), "source-budget.json")
	capped := func() {
		_ = WriteJSON(budget, map[string]any{"day": sourceNow().UTC().Format("2006-01-02"), "runs": 1})
	}
	capped()
	d := noticeDriver(t, log.send)
	d.Gather = noEvidence().Gather
	for i := 0; i < 10; i++ {
		e := RunOrchestratorRound(context.Background(), s, &st, d)
		if !errors.Is(e, ErrSourceDeferred) || st.State != "waiting" || st.Rounds != 0 || len(f.starts) != 0 {
			t.Fatal("cap", i, e, st.State, st.Rounds)
		}
		// Admission refusals in the same day share one daily key.
		orchestratorBudgetNotice(s, "runs_per_day")
		flushOrchestratorNotices(s, log.send)
	}
	if exists(filepath.Join(OrchestratorDir(s.Name), "rounds")) {
		t.Fatal("deferred round created a round directory")
	}
	if log.count("source run cap reached") != 1 || log.count("budget exhausted (runs_per_day)") != 1 || log.count("failure") != 0 || len(log.sent) != 2 {
		t.Fatal(log.sent)
	}
	// A round that runs ends the episode; the next cap starts a new one.
	_ = os.Remove(budget)
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e != nil || st.Rounds != 1 {
		t.Fatal(e, st)
	}
	s.Budget.RunsPerDay = 0
	s.Budget.ConcurrentRuns = 1
	f.listed = map[string]bool{}
	sourceCommand = func(_ context.Context, _ OrchestratorSpec, inv sourceInvocation) ([]byte, error) {
		if inv.Args[0] == "events" {
			return []byte("{\"id\":\"REL-2\",\"kind\":\"issue\",\"url\":\"https://example.com/2\",\"fields\":{\"status\":\"Ready\"}}\n{\"cursor\":\"later\"}\n"), nil
		}
		return []byte(`{"source":true,"ack":true}`), nil
	}
	d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		return []OrchestratorEvidence{{Row: Row{State: "running", Meta: Meta{Order: Order{Name: "busy"}}}}}, nil
	}
	for i := 0; i < 3; i++ {
		if e := RunOrchestratorRound(context.Background(), s, &st, d); !errors.Is(e, ErrSourceDeferred) || st.State != "waiting" {
			t.Fatal("second episode", e, st)
		}
	}
	if log.count("source run cap reached") != 2 {
		t.Fatal(log.sent)
	}
}

// M1: a flood of queued notices and a failing sender cost one attempt per
// recipient per wake, then back off with counted attempts.
func TestNoticeFloodOneAttemptPerRecipient(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	_ = staleOrchestratorNotices(s.Name)
	for i := 0; i < 20; i++ {
		if err := queueOrchestratorNotice(s, "lifecycle", fmt.Sprint("event-", i), "owner", fmt.Sprint("event ", i)); err != nil {
			t.Fatal(err)
		}
		if i < 3 {
			if err := queueOrchestratorNotice(s, "lifecycle", fmt.Sprint("event-", i), "remote/next-owner", fmt.Sprint("event ", i)); err != nil {
				t.Fatal(err)
			}
		}
	}
	perRecipient := map[string]int{}
	fail := func(_ context.Context, n OrchestratorNotice) error {
		perRecipient[n.To]++
		return errors.New("owner unreachable")
	}
	flushOrchestratorNotices(s, fail)
	if len(perRecipient) != 2 || perRecipient["owner"] != 1 || perRecipient["remote/next-owner"] != 1 {
		t.Fatal(perRecipient)
	}
	flushOrchestratorNotices(s, fail)
	if perRecipient["owner"] != 1 {
		t.Fatal("retried inside backoff", perRecipient)
	}
	advanceNotices(2 * time.Minute)
	flushOrchestratorNotices(s, fail)
	var head OrchestratorNotice
	for _, n := range noticesOf(t, s.Name, "lifecycle") {
		if n.Status == "queued" && n.To == "owner" {
			head = n
			break
		}
	}
	if perRecipient["owner"] != 2 || head.Attempts != 2 || !head.NextAttempt.After(noticeNow()) {
		t.Fatal(perRecipient, head)
	}
}

// M2, K9: the pull route queues receipts, never execs a transport, and drains
// through the list and ack RPCs.
func TestPullRouteNoTransportListAck(t *testing.T) {
	orchHome(t)
	old := notificationExecutable
	notificationExecutable = func() (string, error) { t.Fatal("pull route exec'd a transport"); return "", nil }
	t.Cleanup(func() { notificationExecutable = old })
	orchestratorNoticeSender = sendOrchestratorNotice
	raw := strings.Replace(orchestratorExample, "reply: owner", "reply: owner\nnotify_route: pull", 1)
	raw = strings.Replace(raw, "authority: [say,", "authority: [notify, say,", 1)
	s, err := ApplyOrchestrator(raw, false)
	if err != nil || s.NotifyRoute != "pull" {
		t.Fatal(s.NotifyRoute, err)
	}
	st, _ := orchestratorState(s.Name)
	if err = RunOrchestratorRound(context.Background(), s, &st, noticeDriver(t, nil)); err != nil {
		t.Fatal(err)
	}
	if err = OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	r, err := Handle(Request{Action: "orchestrator-notices", Name: s.Name})
	if err != nil || len(r.Notices) != 3 {
		t.Fatal(r.Notices, err)
	}
	for i, want := range []string{"started/applied", "Ready to merge", "stopped"} {
		if !strings.Contains(r.Notices[i].Message, want) || r.Notices[i].Status != "queued" {
			t.Fatal("not oldest first", i, r.Notices)
		}
	}
	if _, err = Handle(Request{Action: "orchestrator-notice-ack", IDs: []string{r.Notices[0].ID, r.Notices[1].ID}}); err != nil {
		t.Fatal(err)
	}
	if r, err = Handle(Request{Action: "orchestrator-notices"}); err != nil || len(r.Notices) != 1 || !strings.Contains(r.Notices[0].Message, "stopped") {
		t.Fatal(r.Notices, err)
	}
	if _, err = Handle(Request{Action: "orchestrator-notice-ack", IDs: []string{"bad"}}); err == nil {
		t.Fatal("accepted an invalid ID")
	}
	if spec, e := ParseOrchestrator(strings.Replace(raw, "notify_route: pull", "notify_route: carrier", 1)); e == nil {
		t.Fatal("accepted unknown route", spec.NotifyRoute)
	}
}

// T (no tmux): a local owner without tmux keeps the notice queued, and the
// pull route still drains it.
func TestNoticeNoTmuxStaysQueuedAndPullDrains(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	noTmux := func(context.Context, OrchestratorNotice) error {
		return errors.New(`exec: "tmux": executable file not found in $PATH`)
	}
	flushOrchestratorNotices(s, noTmux)
	n := noticesOf(t, s.Name, "")
	if len(n) != 1 || n[0].Status != "queued" || !strings.Contains(n[0].Error, "tmux") {
		t.Fatal(n)
	}
	pending, err := PendingOrchestratorNotices("")
	if err != nil || len(pending) != 1 {
		t.Fatal(pending, err)
	}
	if err = AckOrchestratorNotices([]string{pending[0].ID}); err != nil {
		t.Fatal(err)
	}
	if n = noticesOf(t, s.Name, ""); n[0].Status != "delivered" {
		t.Fatal(n)
	}
}

// T (remote recipient): a qualified owner keeps its address and a stable
// HOST/o-NAME sender.
func TestNoticeRemoteRecipient(t *testing.T) {
	orchHome(t)
	raw := strings.Replace(orchestratorExample, "reply: owner", "reply: laptop/owner", 1)
	s, err := ApplyOrchestrator(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	log := &noticeLog{}
	flushOrchestratorNotices(s, log.send)
	if len(log.sent) != 1 || log.sent[0].To != "laptop/owner" || !strings.HasPrefix(log.sent[0].ID, "test-host/o-release-train:lifecycle:") {
		t.Fatal(log.sent)
	}
}

// T (budget notice on a target): a scope-only target never queues or sends.
func TestBudgetNoticeSkippedOnScopeTarget(t *testing.T) {
	orchHome(t)
	s, err := RegisterOrchestratorScope(orchestratorExample, Host{SSH: "owner-host"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = reserveOrchestratorCounts(s, s.Budget.ConcurrentRuns, "budget.json"); err == nil {
		t.Fatal("budget admitted")
	}
	if n := noticesOf(t, s.Name, ""); len(n) != 0 {
		t.Fatal("target queued an owner notice", n)
	}
}

// m2: the sender reuses dispatch's host derivation, and apply rejects a host
// label that peers would refuse.
func TestNoticeSenderHostDerivation(t *testing.T) {
	orchHome(t)
	t.Setenv("PEERS_SELF", "")
	old := hostName
	t.Cleanup(func() { hostName = old })
	hostName = func() (string, error) { return "Build-Box.local", nil }
	s := applyOrch(t)
	n := noticesOf(t, s.Name, "")
	if len(n) != 1 || !strings.HasPrefix(n[0].ID, "build-box/o-release-train:") || !validNoticeSender("build-box/o-release-train") {
		t.Fatal(n)
	}
	hostName = func() (string, error) { return "bad host", nil }
	if _, err := ApplyOrchestrator(orchestratorExample, false); err == nil || !strings.Contains(err.Error(), "sender") {
		t.Fatal("invalid sender accepted", err)
	}
}

// m3: a long round error yields a bounded notice that points to show.
func TestFailureNoticeBounded(t *testing.T) {
	orchHome(t)
	log := &noticeLog{}
	orchestratorNoticeSender = log.send
	s := applyOrch(t)
	st, _ := orchestratorState(s.Name)
	clock := st.Start
	calls := 0
	d := noticeDriver(t, log.send)
	d.Now = func() time.Time { return clock }
	d.Session = func(context.Context, OrchestratorSpec, string, OrchestratorPacket) error {
		calls++
		return errors.New(strings.Repeat("secret-ish detail ", 600))
	}
	d.Wait = func(context.Context) error {
		clock = clock.Add(20 * time.Minute)
		if calls == 2 {
			return os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "stop"), nil, 0600)
		}
		return nil
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, d); err != nil {
		t.Fatal(err)
	}
	pointer := " [truncated; hev orchestrate show " + s.Name + "]"
	for _, n := range log.sent {
		if strings.Contains(n.Message, "consecutive failures") {
			body, ok := strings.CutSuffix(n.Message, pointer)
			if !ok || len([]rune(body)) > noticeTextLimit {
				t.Fatal(len(n.Message), n.Message)
			}
			return
		}
	}
	t.Fatal("no failure notice", log.sent)
}

// m4: a stop that races a round yields one notice.
func TestStopRacingRoundOneNotice(t *testing.T) {
	orchHome(t)
	log := &noticeLog{}
	orchestratorNoticeSender = log.send
	s := applyOrch(t)
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	// The round in flight advanced the round count before the supervisor saw stop.
	st, _ := orchestratorState(s.Name)
	st.Rounds = 3
	if err := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), st); err != nil {
		t.Fatal(err)
	}
	if err := SuperviseOrchestrator(context.Background(), s.Name, noticeDriver(t, log.send)); err != nil {
		t.Fatal(err)
	}
	if log.count("stopped") != 1 {
		t.Fatal(log.sent)
	}
}

// m5: an inbox that refuses a reused ID for a new recipient is terminal.
func TestNoticeReusedIDTerminal(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	calls := 0
	refuse := func(context.Context, OrchestratorNotice) error {
		calls++
		return errors.New("message ID reused with different content or recipient")
	}
	flushOrchestratorNotices(s, refuse)
	advanceNotices(2 * noticeMaxBackoff)
	flushOrchestratorNotices(s, refuse)
	if n := noticesOf(t, s.Name, ""); calls != 1 || len(n) != 1 || n[0].Status != "failed" {
		t.Fatal(calls, n)
	}
}

// m6: reapply marks queued notices stale so none queued before a stop is sent.
func TestReapplyStalesQueuedNotices(t *testing.T) {
	orchHome(t)
	orchestratorNoticeSender = func(context.Context, OrchestratorNotice) error { return errors.New("owner unreachable") }
	s := applyOrch(t)
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	log := &noticeLog{}
	orchestratorNoticeSender = log.send
	s = applyOrch(t)
	advanceNotices(2 * noticeMaxBackoff)
	flushOrchestratorNotices(s, log.send)
	if len(log.sent) != 1 || !strings.Contains(log.sent[0].Message, "started/applied") {
		t.Fatal(log.sent)
	}
	stale := 0
	for _, n := range noticesOf(t, s.Name, "") {
		if n.Status == "stale" {
			stale++
		}
	}
	if stale != 2 {
		t.Fatal("old lineage notices not stale", noticesOf(t, s.Name, ""))
	}
}

// n1: a notice rename is followed by an fsync of its directory.
func TestWriteNoticeSyncsParentDir(t *testing.T) {
	dir := t.TempDir()
	old := noticeDirSync
	t.Cleanup(func() { noticeDirSync = old })
	var synced []string
	noticeDirSync = func(d string) error { synced = append(synced, d); return old(d) }
	if err := writeNotice(filepath.Join(dir, "n.json"), &OrchestratorNotice{ID: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0] != dir {
		t.Fatal(synced)
	}
}

// K12: intake signals queue owner notices through the #155 hook, once per
// receipt across a crash between queueing and delivered, with a stable ID.
func TestIntakeSignalNoticeCrashRedelivery(t *testing.T) {
	s := sourceFixture(t)
	key := "receipt-1"
	sig := IntakeSignal{Kind: "rejected", Receipt: key, Event: "REL-9", Error: "target refused", At: time.Now().UTC(), ID: signalID("rejected", key)}
	if err := os.MkdirAll(sourceMarkers(s), 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(sourceMarkers(s), key+".json"), sourceReceipt{Mode: "rejected", Event: "REL-9", Signals: []IntakeSignal{sig}}); err != nil {
		t.Fatal(err)
	}
	s.PauseFile = filepath.Join(t.TempDir(), "PAUSED")
	_ = os.WriteFile(s.PauseFile, nil, 0600)
	log := &noticeLog{fail: func(OrchestratorNotice) error { return errors.New("owner unreachable") }}
	flushOrchestratorNotices(s, func(context.Context, OrchestratorNotice) error { return nil })
	// Round 1: the notice is queued, then the process dies before the outbox
	// records delivered.
	old := IntakeSignalHook
	t.Cleanup(func() { IntakeSignalHook = old })
	IntakeSignalHook = func(s OrchestratorSpec, sig IntakeSignal) error {
		if err := queueIntakeNotice(s, sig); err != nil {
			return err
		}
		return errors.New("crash before delivered")
	}
	st, _ := orchestratorState(s.Name)
	d := noticeDriver(t, log.send)
	d.Gather = noEvidence().Gather
	if e := RunOrchestratorRound(context.Background(), s, &st, d); !errors.Is(e, ErrSourceDeferred) {
		t.Fatal(e)
	}
	queued := noticesOf(t, s.Name, "intake")
	if len(queued) != 1 || queued[0].Status != "queued" || !strings.Contains(queued[0].Message, "intake rejected for event REL-9") {
		t.Fatal(queued)
	}
	// Round 2, later: redelivery reuses the receipt, and the owner gets it once.
	IntakeSignalHook = old
	log.fail = nil
	advanceNotices(2 * noticeMaxBackoff)
	for i := 0; i < 2; i++ {
		if e := RunOrchestratorRound(context.Background(), s, &st, d); !errors.Is(e, ErrSourceDeferred) {
			t.Fatal(e)
		}
	}
	var intake []OrchestratorNotice
	for _, n := range log.sent {
		if n.Kind == "intake" {
			intake = append(intake, n)
		}
	}
	if len(intake) != 2 || intake[0].ID != queued[0].ID || intake[1].ID != queued[0].ID || noticesOf(t, s.Name, "intake")[0].Status != "delivered" {
		t.Fatal("want one failed then one delivered attempt with the queued ID", intake)
	}
	var rc sourceReceipt
	if err := readJSON(filepath.Join(sourceMarkers(s), key+".json"), &rc); err != nil || len(rc.Delivered) != 1 {
		t.Fatal(rc, err)
	}
}

// K12: a hook whose queue write fails returns an error and the signal stays
// pending; the hook never sends.
func TestIntakeSignalHookQueueFailure(t *testing.T) {
	s := sourceFixture(t)
	key := "receipt-2"
	sig := IntakeSignal{Kind: "needs-reconcile", Receipt: key, Event: "REL-3", ID: signalID("needs-reconcile", key)}
	_ = os.MkdirAll(sourceMarkers(s), 0700)
	if err := WriteJSON(filepath.Join(sourceMarkers(s), key+".json"), sourceReceipt{Mode: "needs-reconcile", Event: "REL-3", Signals: []IntakeSignal{sig}}); err != nil {
		t.Fatal(err)
	}
	notices := filepath.Join(OrchestratorDir(s.Name), "notices")
	_ = os.MkdirAll(notices, 0700)
	if err := os.WriteFile(filepath.Join(notices, "intake"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	orchestratorNoticeSender = func(context.Context, OrchestratorNotice) error { t.Fatal("hook sent"); return nil }
	if err := IntakeSignalHook(s, sig); err == nil {
		t.Fatal("queue failure acknowledged")
	}
	if err := deliverSourceSignals(s, nil); err != nil {
		t.Fatal(err)
	}
	var rc sourceReceipt
	if err := readJSON(filepath.Join(sourceMarkers(s), key+".json"), &rc); err != nil || len(rc.Delivered) != 0 {
		t.Fatal("signal acknowledged without a queued notice", rc, err)
	}
	_ = os.Remove(filepath.Join(notices, "intake"))
	if err := deliverSourceSignals(s, nil); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(filepath.Join(sourceMarkers(s), key+".json"), &rc); err != nil || len(rc.Delivered) != 1 {
		t.Fatal(rc, err)
	}
}

// K10: a seed checkpoint is validated and recorded as round 000 with
// provenance, and round 001 reads it.
func TestSeedCheckpoint(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	file := filepath.Join(t.TempDir(), "seed.md")
	if err := os.WriteFile(file, []byte("Notes only.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SeedOrchestratorCheckpoint(s.Name, file); err == nil || !strings.Contains(err.Error(), "goal status") {
		t.Fatal("accepted a checkpoint without sections", err)
	}
	seed := "## Goal status\nPending.\n## Open threads\n- REL-1: waiting on review\n## Next action\nCheck REL-1.\n"
	if err := os.WriteFile(file, []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SeedOrchestratorCheckpoint(s.Name, file); err != nil {
		t.Fatal(err)
	}
	round := filepath.Join(OrchestratorDir(s.Name), "rounds", "000")
	var prov map[string]string
	if err := readJSON(filepath.Join(round, "provenance.json"), &prov); err != nil || prov["source"] != file || len(prov["sha256"]) != 64 || prov["time"] == "" {
		t.Fatal(prov, err)
	}
	if b, _ := os.ReadFile(filepath.Join(round, "checkpoint.md")); string(b) != seed {
		t.Fatal(string(b))
	}
	if err := SeedOrchestratorCheckpoint(s.Name, file); err != nil {
		t.Fatal("same seed is idempotent", err)
	}
	st, _ := orchestratorState(s.Name)
	var packet OrchestratorPacket
	d := noticeDriver(t, nil)
	session := d.Session
	d.Session = func(ctx context.Context, s OrchestratorSpec, dir string, p OrchestratorPacket) error {
		packet = p
		return session(ctx, s, dir, p)
	}
	if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil || packet.Checkpoint != seed || st.Rounds != 1 {
		t.Fatal(err, packet.Checkpoint, st.Rounds)
	}
	_ = os.WriteFile(file, []byte(seed+"More.\n"), 0600)
	if err := SeedOrchestratorCheckpoint(s.Name, file); err == nil {
		t.Fatal("reseeded after round 001")
	}
}

// F13: the docs state the trust boundary of a remote orchestrator-apply.
func TestOrchestratorApplyRPCBoundaryDocumented(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "orchestrators.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "`orchestrator-apply`") || !strings.Contains(string(b), "any caller that can run `hev dispatch rpc`") {
		t.Fatal("orchestrator-apply trust boundary is undocumented")
	}
}
