//go:build !windows

package businessflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

type flow02Ledger struct {
	t  *testing.T
	s  Store
	st Snapshot
}

func newFlow02Ledger(t *testing.T) *flow02Ledger {
	t.Helper()
	f := &flow02Ledger{t: t, s: Store{Dir: t.TempDir(), Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}}
	f.apply(Command{Action: "policy", Reason: "synthetic policy", Policy: Policy{Execution: 2, Validation: 1, BacklogHigh: 4, BacklogLow: 1, BacklogHard: 4, AcceptanceHigh: 2, AcceptanceLow: 0}})
	return f
}
func (f *flow02Ledger) command(c Command) Command {
	c.ExpectedVersion = f.st.ResourceVersion
	c.Key = fmt.Sprint("fixture-", f.st.ResourceVersion)
	c.Actor = "synthetic-chef"
	c.Authority = "synthetic-fence"
	if c.Order != "" && c.Obligation == "" {
		c.Obligation = "obligation-" + c.Order
	}
	return c
}
func (f *flow02Ledger) apply(c Command) {
	f.t.Helper()
	st, err := f.s.Apply(context.Background(), f.command(c))
	if err != nil {
		f.t.Fatalf("expected %s success; actual %v", c.Action, err)
	}
	f.st = st
}
func (f *flow02Ledger) reject(c Command) {
	f.t.Helper()
	_, err := f.s.Apply(context.Background(), f.command(c))
	if err == nil {
		f.t.Fatalf("expected %s rejected; actual success", c.Action)
	}
	after, e := f.s.Snapshot(context.Background())
	if e != nil || after.ResourceVersion != f.st.ResourceVersion {
		f.t.Fatalf("expected rejection without mutation; actual revision %d err=%v", after.ResourceVersion, e)
	}
}
func flow02Reservation(id string) Reservation {
	return Reservation{ID: id, Attempt: id, Executor: "fixture-node", Intent: id, Units: 1}
}
func (f *flow02Ledger) commit(id string) {
	f.apply(Command{Action: "commit", Order: id, Owner: "retained-worker", Scope: "scope"})
}
func (f *flow02Ledger) reserve(action, order, id, candidate string) {
	f.apply(Command{Action: action, Order: order, Reservation: flow02Reservation(id), Candidate: candidate})
}
func (f *flow02Ledger) settle(order, id string) {
	f.apply(Command{Action: "settle", Order: order, Reservation: Reservation{ID: id}, Reason: "synthetic observed effect", Evidence: "fixture:effect"})
}
func (f *flow02Ledger) candidate(order, candidate string) {
	f.apply(Command{Action: "candidate", Order: order, Candidate: candidate, Evidence: "fixture:manifest"})
}
func (f *flow02Ledger) result(order, candidate, result string) {
	f.apply(Command{Action: "result", Order: order, Candidate: candidate, Validator: "independent-fixture", Result: result, Evidence: "fixture:assertion"})
}

func TestFlow02StableObligationAndDelivery(t *testing.T) {
	f := newFlow02Ledger(t)
	f.commit("o")
	f.reserve("start", "o", "initial", "")
	f.settle("o", "initial")
	f.candidate("o", "A")
	initial := f.st.Entries["o"]
	f.reserve("validate", "o", "validate-A", "A")
	f.settle("o", "validate-A")
	f.result("o", "A", "Failed")
	f.reserve("rework", "o", "rework", "")
	f.settle("o", "rework")
	f.candidate("o", "B")
	f.reserve("validate", "o", "validate-B", "B")
	f.settle("o", "validate-B")
	f.reject(Command{Action: "result", Order: "o", Candidate: "A", Validator: "independent-fixture", Result: "Passed", Evidence: "fixture:late"})
	f.reject(Command{Action: "deliver", Order: "o", Candidate: "B", Delivery: "fixture:delivery", Evidence: "fixture:observed"})
	f.result("o", "B", "Passed")
	if rows := ordered(f.st, Acceptance); len(rows) != 1 {
		t.Fatalf("expected pass awaiting delivery still in Acceptance; actual %v", rows)
	}
	f.reject(Command{Action: "deliver", Order: "o", Candidate: "A", Delivery: "fixture:delivery", Evidence: "fixture:observed"})
	f.apply(Command{Action: "deliver", Order: "o", Candidate: "B", Delivery: "fixture:delivery", Evidence: "fixture:observed"})
	e := f.st.Entries["o"]
	if e.Obligation != initial.Obligation || e.Owner != initial.Owner || e.Rank != initial.Rank || e.Sequence != initial.Sequence || e.AcceptanceSequence != initial.AcceptanceSequence || !e.AcceptanceEnteredAt.Equal(initial.AcceptanceEnteredAt) || len(e.History) != 2 || e.History[0].Result != "Failed" || e.History[1].Result != "Passed" || len(ordered(f.st, Acceptance)) != 0 {
		t.Fatalf("expected stable obligation/history and exit only after delivery; actual %+v", e)
	}
	t.Log("expected=same obligation/worker/rank/age through A failure and B pass; late A rejected, pass remains queued until delivery; actual=matched")
}

func TestFlow02QueueOrderingWIPAndPressure(t *testing.T) {
	f := newFlow02Ledger(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		f.commit(id)
	}
	f.reject(Command{Action: "commit", Order: "e", Owner: "worker", Scope: "scope"})
	f.reject(Command{Action: "start", Order: "b", Reservation: flow02Reservation("skip-a")})
	for _, id := range []string{"a", "b"} {
		f.reserve("start", id, "start-"+id, "")
	}
	f.reject(Command{Action: "start", Order: "c", Reservation: flow02Reservation("over-wip")})
	// Reverse completion arrival must preserve original committed business order.
	for _, id := range []string{"b", "a"} {
		f.settle(id, "start-"+id)
		f.candidate(id, "candidate-"+id)
	}
	rows := ordered(f.st, Acceptance)
	if len(rows) != 2 || rows[0].Order != "a" || rows[1].Order != "b" || !f.st.StartsPaused {
		t.Fatalf("expected a,b and paused at high crossing; actual %+v paused=%v", rows, f.st.StartsPaused)
	}
	f.s = Store{Dir: f.s.Dir, Gate: f.s.Gate}
	var err error
	f.st, err = f.s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.reject(Command{Action: "start", Order: "c", Reservation: flow02Reservation("pressure")})
	f.reject(Command{Action: "validate", Order: "b", Candidate: "candidate-b", Reservation: flow02Reservation("skip-validation")})
	for _, id := range []string{"a", "b"} {
		f.reserve("validate", id, "validate-"+id, "candidate-"+id)
		if id == "a" { // A live validation slot cannot be reused, even with recorded bypass.
			f.reject(Command{Action: "validate", Order: "b", Candidate: "candidate-b", Reservation: flow02Reservation("over-validation"), Bypasses: []Bypass{{Order: "a", Reason: "claimed", Evidence: "fixture:claim", Owner: "validator", Release: "result"}}})
		}
		f.settle(id, "validate-"+id)
		f.result(id, "candidate-"+id, "Passed")
		f.apply(Command{Action: "deliver", Order: id, Candidate: "candidate-" + id, Delivery: "fixture:delivery", Evidence: "fixture:observed"})
		if id == "a" && !f.st.StartsPaused {
			t.Fatal("expected latch closed in midband; actual open")
		}
	}
	if f.st.StartsPaused {
		t.Fatal("expected reopen at low crossing; actual paused")
	}
	f.reserve("start", "c", "released", "")
	// Without a trusted capacity/authority gate the ledger never admits a mutation.
	f.s.Gate = nil
	f.reject(Command{Action: "start", Order: "d", Reservation: flow02Reservation("no-authority")})
	t.Log("expected=FIFO despite reverse arrivals, bounded execution/validation, durable high/mid/low pressure and unknown authority held; actual=matched; installed integration remains unsupported/pending")
}

// Real SIGKILL occurs after the fsynced receiver intent and on either side of
// the synthetic external effect. The parent starts a fresh process/store view.
func TestFlow02ReceiverKillBoundaries(t *testing.T) {
	if root := os.Getenv("FLOW02_KILL_ROOT"); root != "" {
		s := Store{Dir: root}
		binding := ReservationCheck{Order: "o", Obligation: "obligation-o", Scope: "scope", Attempt: "r", Intent: "r", Executor: "fixture-node", PolicyRevision: 1}
		err := s.WithReservationGuard(context.Background(), "r", binding, func(context.Context) error {
			st, err := s.Snapshot(context.Background())
			if err != nil || st.Reservations["r"].Status != "Starting" {
				return fmt.Errorf("intent not durable: %v", err)
			}
			if os.Getenv("FLOW02_KILL_BOUNDARY") == "after-effect" {
				if err := os.WriteFile(filepath.Join(root, "effect"), []byte("one\n"), 0600); err != nil {
					return err
				}
			}
			return syscall.Kill(os.Getpid(), syscall.SIGKILL)
		})
		t.Fatalf("expected SIGKILL; actual returned %v", err)
	}
	for _, boundary := range []string{"before-effect", "after-effect"} {
		t.Run(boundary, func(t *testing.T) {
			f := newFlow02Ledger(t)
			f.commit("o")
			f.reserve("start", "o", "r", "")
			child := exec.Command(os.Args[0], "-test.run=^TestFlow02ReceiverKillBoundaries$")
			child.Env = append(os.Environ(), "FLOW02_KILL_ROOT="+f.s.Dir, "FLOW02_KILL_BOUNDARY="+boundary)
			err := child.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ProcessState.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
				t.Fatalf("expected SIGKILL; actual %v", err)
			}
			restarted := Store{Dir: f.s.Dir}
			st, err := restarted.Snapshot(context.Background())
			if err != nil || st.Reservations["r"].Status != "Starting" {
				t.Fatalf("expected uncertain Starting intent; actual %+v %v", st.Reservations, err)
			}
			binding := ReservationCheck{Order: "o", Obligation: "obligation-o", Scope: "scope", Attempt: "r", Intent: "r", Executor: "fixture-node", PolicyRevision: 1}
			calls := 0
			for range 2 {
				err = restarted.WithReservationGuard(context.Background(), "r", binding, func(context.Context) error { calls++; return nil })
				if !errors.Is(err, ErrHeld) {
					t.Fatalf("expected ErrHeld; actual %v", err)
				}
			}
			data, err := os.ReadFile(filepath.Join(f.s.Dir, "effect"))
			want := ""
			if boundary == "after-effect" {
				want = "one\n"
			}
			if calls != 0 || string(data) != want || err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("expected no duplicate, effect=%q; actual calls=%d effect=%q err=%v", want, calls, data, err)
			}
			t.Logf("expected=uncertain intent and no replay after %s SIGKILL; actual=Starting (charged/held), retry calls=0, effect=%q", boundary, data)
		})
	}
}
