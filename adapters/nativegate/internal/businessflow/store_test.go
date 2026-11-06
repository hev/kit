//go:build !windows

package businessflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestLifecycleRestartAndRework(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	rev := uint64(0)
	apply := func(c Command) Snapshot {
		t.Helper()
		c.ExpectedVersion = rev
		c.Key = string(rune('a' + rev))
		c.Actor = "chef"
		c.Authority = "fence:1"
		v, e := s.Apply(ctx, c)
		if e != nil {
			t.Fatalf("%s: %v", c.Action, e)
		}
		rev = v.ResourceVersion
		return v
	}
	apply(Command{Action: "policy", Reason: "test", Policy: Policy{Execution: 2, Validation: 1, ReworkProtected: 1, BacklogHigh: 3, BacklogLow: 1, BacklogHard: 3, AcceptanceHigh: 2, AcceptanceLow: 0}})
	apply(Command{Action: "commit", Order: "o", Obligation: "a", Owner: "worker", Scope: "s"})
	reservation := func(id string) Reservation {
		return Reservation{ID: id, Attempt: id, Executor: "n", Intent: id, Units: 1}
	}
	apply(Command{Action: "start", Order: "o", Obligation: "a", Reservation: reservation("r1")})
	apply(Command{Action: "unknown", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r1"}, Reason: "lost ACK", Evidence: "query"})
	bad := Command{ExpectedVersion: rev, Key: "bad", Actor: "chef", Authority: "fence:1", Action: "candidate", Order: "o", Obligation: "a", Candidate: "c1", Evidence: "manifest"}
	if _, e := s.Apply(ctx, bad); !errors.Is(e, ErrHeld) {
		t.Fatal(e)
	}
	apply(Command{Action: "settle", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r1"}, Reason: "observed", Evidence: "done"})
	first := apply(Command{Action: "candidate", Order: "o", Obligation: "a", Candidate: "c1", Evidence: "manifest"})
	for i, candidate := range []string{"c1", "c2"} {
		id := candidate + "v"
		apply(Command{Action: "validate", Order: "o", Obligation: "a", Candidate: candidate, Reservation: reservation(id)})
		apply(Command{Action: "settle", Order: "o", Obligation: "a", Reservation: Reservation{ID: id}, Reason: "observed", Evidence: "done"})
		result := "Failed"
		if i == 1 {
			result = "Passed"
		}
		apply(Command{Action: "result", Order: "o", Obligation: "a", Candidate: candidate, Validator: "independent", Result: result, Evidence: "checks"})
		if i == 0 {
			apply(Command{Action: "rework", Order: "o", Obligation: "a", Reservation: reservation("r2")})
			apply(Command{Action: "settle", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r2"}, Reason: "observed", Evidence: "done"})
			apply(Command{Action: "candidate", Order: "o", Obligation: "a", Candidate: "c2", Evidence: "manifest2"})
		}
	}
	final := apply(Command{Action: "deliver", Order: "o", Obligation: "a", Candidate: "c2", Delivery: "observed-release", Evidence: "receipt"})
	e := final.Entries["o"]
	if e.EnteredAt.IsZero() || e.AcceptanceEnteredAt.IsZero() || !e.EnteredAt.Equal(first.Entries["o"].EnteredAt) || !e.AcceptanceEnteredAt.Equal(first.Entries["o"].AcceptanceEnteredAt) {
		t.Fatal("rework changed age", e)
	}
	if e.Obligation != "a" || len(e.History) != 2 || e.Sequence != first.Entries["o"].Sequence || e.AcceptanceSequence != first.Entries["o"].AcceptanceSequence {
		t.Fatal(e)
	}
	restarted, err := (Store{Dir: s.Dir}).Snapshot(ctx)
	if err != nil || restarted.ResourceVersion != rev || restarted.Entries["o"].Phase != "Delivered" {
		t.Fatal(restarted, err)
	}
}
func TestConcurrentCASAndGate(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	p := Policy{Execution: 2, Validation: 1, BacklogHigh: 2, BacklogLow: 0, BacklogHard: 2, AcceptanceHigh: 2, AcceptanceLow: 0}
	_, err := s.Apply(ctx, Command{Key: "p", Actor: "a", Authority: "f", Action: "policy", Reason: "test", Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wins := make(chan bool, 2)
	for _, id := range []string{"one", "two"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, e := s.Apply(ctx, Command{ExpectedVersion: 1, Key: id, Actor: "a", Authority: "f", Action: "commit", Order: id, Obligation: id, Owner: "w", Scope: "s"})
			wins <- e == nil
		}(id)
	}
	wg.Wait()
	close(wins)
	n := 0
	for v := range wins {
		if v {
			n++
		}
	}
	if n != 1 {
		t.Fatal(n)
	}
	s.Gate = GateFunc(func(context.Context, Command, Snapshot) error { return ErrHeld })
	_, err = s.Apply(ctx, Command{ExpectedVersion: 2, Key: "held", Actor: "a", Authority: "f", Action: "policy", Reason: "test", Policy: p})
	if !errors.Is(err, ErrHeld) {
		t.Fatal(err)
	}
}

func TestPressureIsDurableAndIdempotent(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	p := Policy{Execution: 3, Validation: 1, BacklogHigh: 2, BacklogLow: 0, BacklogHard: 2, AcceptanceHigh: 2, AcceptanceLow: 0}
	c := Command{Key: "policy", Actor: "a", Authority: "f", Action: "policy", Reason: "test", Policy: p}
	st, err := s.Apply(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1", "2"} {
		c = Command{ExpectedVersion: st.ResourceVersion, Key: id, Actor: "a", Authority: "f", Action: "commit", Order: id, Obligation: id, Owner: "w", Scope: "s"}
		st, err = s.Apply(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !st.CommitmentsPaused {
		t.Fatal("high crossing did not close")
	}
	replayed, err := s.Apply(ctx, c)
	if err != nil || replayed.ResourceVersion != st.ResourceVersion {
		t.Fatal("lost ACK replay", err)
	}
	c.Scope = "changed"
	if _, err = s.Apply(ctx, c); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	st, err = (Store{Dir: s.Dir}).Snapshot(ctx)
	if err != nil || !st.CommitmentsPaused {
		t.Fatal("restart opened latch", err)
	}
	_, err = s.Apply(ctx, Command{ExpectedVersion: st.ResourceVersion, Key: "third", Actor: "a", Authority: "f", Action: "commit", Order: "3", Obligation: "3", Owner: "w", Scope: "s"})
	if !errors.Is(err, ErrHeld) {
		t.Fatal(err)
	}
}

func TestCanceledGateCannotPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { cancel(); return nil })}
	_, err := s.Apply(ctx, Command{Key: "p", Actor: "a", Authority: "f", Action: "policy", Reason: "test", Policy: Policy{Execution: 1, Validation: 1, BacklogHigh: 2, BacklogLow: 0, BacklogHard: 2, AcceptanceHigh: 2, AcceptanceLow: 0}})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = s.Snapshot(context.Background()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled mutation published", err)
	}
}
func TestSnapshotRejectsFIFOAndSymlink(t *testing.T) {
	for _, kind := range []string{"fifo", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			if kind == "fifo" {
				if err := syscall.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Symlink("target", path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := (Store{Dir: dir}).Snapshot(context.Background()); err == nil {
				t.Fatal("accepted special file")
			}
		})
	}
}
func TestCancelAndScopeRetainObligation(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	var st Snapshot
	apply := func(c Command) {
		t.Helper()
		c.ExpectedVersion = st.ResourceVersion
		c.Key = fmt.Sprint(st.ResourceVersion)
		c.Actor = "a"
		c.Authority = "f"
		var err error
		st, err = s.Apply(ctx, c)
		if err != nil {
			t.Fatal(c.Action, err)
		}
	}
	apply(Command{Action: "policy", Reason: "test", Policy: Policy{Execution: 1, Validation: 1, BacklogHigh: 2, BacklogLow: 0, BacklogHard: 2, AcceptanceHigh: 2, AcceptanceLow: 0}})
	apply(Command{Action: "commit", Order: "o", Obligation: "a", Owner: "w", Scope: "s1"})
	apply(Command{Action: "revise-scope", Order: "o", Obligation: "a", Scope: "s2", Reason: "reviewed revision", Evidence: "review"})
	apply(Command{Action: "start", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r", Attempt: "try", Executor: "n", Intent: "i", Units: 1}})
	apply(Command{Action: "cancel", Order: "o", Obligation: "a", Reason: "withdrawn", Evidence: "authority"})
	if st.Entries["o"].Phase != "CancellationHold" {
		t.Fatal(st)
	}
	apply(Command{Action: "settle", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r"}, Reason: "observed", Evidence: "done"})
	apply(Command{Action: "cancel", Order: "o", Obligation: "a", Reason: "withdrawn", Evidence: "authority"})
	e := st.Entries["o"]
	if e.Phase != "Cancelled" || e.Obligation != "a" || e.Scope != "s2" || len(e.Scopes) != 1 || e.Scopes[0].Scope != "s1" {
		t.Fatal(e)
	}
}

func TestReceiverPersistsBeforeEffectAndNeverReplays(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	var st Snapshot
	apply := func(c Command) {
		t.Helper()
		c.ExpectedVersion = st.ResourceVersion
		c.Key = fmt.Sprint(st.ResourceVersion)
		c.Actor = "a"
		c.Authority = "f"
		var err error
		st, err = s.Apply(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}
	apply(Command{Action: "policy", Reason: "test", Policy: Policy{Execution: 1, Validation: 1, BacklogHigh: 2, BacklogLow: 0, BacklogHard: 2, AcceptanceHigh: 2, AcceptanceLow: 0}})
	apply(Command{Action: "commit", Order: "o", Obligation: "a", Owner: "w", Scope: "s"})
	apply(Command{Action: "start", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r", Attempt: "try", Executor: "n", Intent: "i", Units: 1}})
	binding := ReservationCheck{Order: "o", Obligation: "a", Scope: "s", Attempt: "try", Intent: "i", Executor: "n", PolicyRevision: 1}
	calls := 0
	effectErr := errors.New("unknown ACK")
	err := s.WithReservationGuard(ctx, "r", binding, func(context.Context) error {
		calls++
		snap, e := s.Snapshot(ctx)
		if e != nil || snap.Reservations["r"].Status != "Starting" {
			t.Fatal("intent not persisted", e)
		}
		return effectErr
	})
	if !errors.Is(err, effectErr) {
		t.Fatal(err)
	}
	snap, err := s.Snapshot(ctx)
	if err != nil || snap.Reservations["r"].Status != "Unknown" {
		t.Fatal(snap, err)
	}
	err = s.WithReservationGuard(ctx, "r", binding, func(context.Context) error { calls++; return nil })
	if !errors.Is(err, ErrHeld) || calls != 1 {
		t.Fatal(err, calls)
	}
}

func TestViewRestartPaginationAndUnknown(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	var st Snapshot
	apply := func(c Command) {
		t.Helper()
		c.ExpectedVersion = st.ResourceVersion
		c.Key = fmt.Sprint(st.ResourceVersion)
		c.Actor = "a"
		c.Authority = "f"
		var err error
		st, err = s.Apply(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}
	apply(Command{Action: "policy", Reason: "test", Policy: Policy{Execution: 2, Validation: 1, BacklogHigh: 3, BacklogLow: 0, BacklogHard: 3, AcceptanceHigh: 2, AcceptanceLow: 0}})
	for _, id := range []string{"a", "b"} {
		apply(Command{Action: "commit", Order: id, Obligation: id, Owner: "w", Scope: "s"})
	}
	original := st.Entries["a"].EnteredAt
	apply(Command{Action: "reorder", Order: "a", Priority: 60, Rank: 1, Reason: "reviewed"})
	restarted := Store{Dir: s.Dir}
	v, err := restarted.View(ctx, Backlog, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.Total != 2 || len(v.Rows) != 1 || v.NextOffset == nil || *v.NextOffset != 1 || v.Rows[0].Position != 1 || v.Rows[0].Entry.Order != "a" || v.Rows[0].Eligibility != "Unknown" || !v.Rows[0].EnteredAt.Equal(original) {
		t.Fatal(v)
	}
	next, err := restarted.View(ctx, Backlog, *v.NextOffset, 1)
	if err != nil || next.ResourceVersion != v.ResourceVersion || next.Rows[0].Position != 2 || next.Rows[0].Entry.Order != "b" || next.NextOffset != nil {
		t.Fatal(next, err)
	}
	if _, err = restarted.View(ctx, Backlog, 0, 101); err == nil {
		t.Fatal("unbounded view")
	}
	missing := Store{Dir: filepath.Join(t.TempDir(), "missing")}
	if _, err = missing.View(ctx, Backlog, 0, 1); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err = os.Stat(missing.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read initialized directory", err)
	}
}

func TestAdversarialUnitsAndHistoricalBypass(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	var st Snapshot
	apply := func(c Command) {
		t.Helper()
		c.ExpectedVersion = st.ResourceVersion
		c.Key = fmt.Sprint(st.ResourceVersion)
		c.Actor = "a"
		c.Authority = "f"
		var err error
		st, err = s.Apply(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}
	p := Policy{Execution: 2, Validation: 1, BacklogHigh: 3, BacklogLow: 0, BacklogHard: 3, AcceptanceHigh: 2, AcceptanceLow: 0}
	apply(Command{Action: "policy", Reason: "test", Policy: p})
	for _, id := range []string{"a", "b"} {
		apply(Command{Action: "commit", Order: id, Obligation: id, Owner: "w", Scope: "s"})
	}
	maxInt := int(^uint(0) >> 1)
	bad := Command{ExpectedVersion: st.ResourceVersion, Key: "overflow", Actor: "a", Authority: "f", Action: "start", Order: "a", Obligation: "a", Reservation: Reservation{ID: "bad", Attempt: "t", Executor: "n", Intent: "i", Units: maxInt}}
	if _, err := s.Apply(ctx, bad); err == nil {
		t.Fatal("accepted MaxInt capacity")
	}
	huge := p
	huge.Execution = maxInt
	bad.Action = "policy"
	bad.Policy = huge
	bad.Reason = "bad bound"
	bad.Key = "hugepolicy"
	if _, err := s.Apply(ctx, bad); err == nil {
		t.Fatal("accepted unbounded policy")
	}
	apply(Command{Action: "start", Order: "b", Obligation: "b", Reservation: Reservation{ID: "r", Attempt: "t", Executor: "n", Intent: "i", Units: 1}, Bypasses: []Bypass{{Order: "a", Reason: "platform unavailable", Evidence: "old-observation", Owner: "operator", Release: "fresh enrollment"}}})
	view, err := s.View(ctx, Backlog, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Rows) != 1 || view.Rows[0].Eligibility != "Unknown" || !strings.HasPrefix(view.Rows[0].Reason, "historical recorded bypass: ") || view.Rows[0].BypassRevision == 0 {
		t.Fatal(view)
	}
}

func TestQueueViewAtomicChargedSummary(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	var st Snapshot
	apply := func(c Command) {
		t.Helper()
		c.ExpectedVersion = st.ResourceVersion
		c.Key = fmt.Sprint(st.ResourceVersion)
		c.Actor = "a"
		c.Authority = "f"
		var err error
		st, err = s.Apply(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
	}
	w := WorkspaceIdentity{UID: "customer", Incarnation: "vm1"}
	p := Policy{Execution: 4, Validation: 2, BacklogHigh: 4, BacklogLow: 0, BacklogHard: 4, AcceptanceHigh: 2, AcceptanceLow: 0, WorkspaceQuotas: []WorkspaceQuota{{Workspace: w, Commitments: 4, Execution: 4, Validation: 2}}}
	apply(Command{Action: "policy", Reason: "test", Policy: p})
	apply(Command{Action: "commit", Order: "o", Obligation: "a", Owner: "w", Scope: "s", Workspace: w})
	apply(Command{Action: "start", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r", Attempt: "t", Executor: "n", Intent: "i", Units: 2, Workspace: w}})
	apply(Command{Action: "unknown", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r"}, Reason: "lost ACK", Evidence: "query"})
	v, err := s.View(ctx, Backlog, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	summary := v.Summary
	if v.ResourceVersion != st.ResourceVersion || summary.BacklogCount != v.Total || summary.Execution.ChargedCount != 1 || summary.Execution.ChargedUnits != 2 || summary.Policy.Execution != 4 || len(summary.Workspaces) != 1 || summary.Workspaces[0].Execution.ChargedUnits != 2 || summary.Workspaces[0].Commitments != 1 || !summary.Workspaces[0].Configured {
		t.Fatal(v)
	}
	found := false
	for _, state := range summary.Execution.States {
		if state.State == "Unknown" && state.ChargedCount == 1 && state.ChargedUnits == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("unknown reservation not charged", summary)
	}
	apply(Command{Action: "settle", Order: "o", Obligation: "a", Reservation: Reservation{ID: "r"}, Reason: "observed", Evidence: "done"})
	v, err = s.View(ctx, Backlog, 0, 10)
	if err != nil || v.Summary.Execution.ChargedUnits != 0 {
		t.Fatal(v, err)
	}
}
func TestQueueViewSummaryConcurrentRevision(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	p := Policy{Execution: 2, Validation: 1, BacklogHigh: 100, BacklogLow: 0, BacklogHard: 100, AcceptanceHigh: 2, AcceptanceLow: 0}
	st, err := s.Apply(ctx, Command{Key: "p", Actor: "a", Authority: "f", Action: "policy", Reason: "test", Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 30; i++ {
			id := fmt.Sprint(i)
			var e error
			st, e = s.Apply(ctx, Command{ExpectedVersion: st.ResourceVersion, Key: id, Actor: "a", Authority: "f", Action: "commit", Order: id, Obligation: id, Owner: "w", Scope: "s"})
			if e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	for {
		v, e := s.View(ctx, Backlog, 0, 100)
		if e != nil {
			t.Fatal(e)
		}
		if v.Summary.BacklogCount != v.Total || len(v.Rows) != v.Total || v.ResourceVersion != uint64(v.Total+1) {
			t.Fatal("mixed snapshot summary", v)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			return
		default:
		}
	}
}
