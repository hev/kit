//go:build !windows

package businessflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestWorkspaceQuotaRestartUnknownAndIndependentProgress(t *testing.T) {
	ctx := context.Background()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := Store{Dir: dir, Gate: GateFunc(func(context.Context, Command, Snapshot) error { return nil })}
	rev := uint64(0)
	key := 0
	call := func(c Command) (Snapshot, error) {
		key++
		c.Key = fmt.Sprint(key)
		c.Actor = "chef"
		c.Authority = "current-fence"
		c.ExpectedVersion = rev
		v, e := s.Apply(ctx, c)
		if e == nil {
			rev = v.ResourceVersion
		}
		return v, e
	}
	must := func(c Command) Snapshot {
		t.Helper()
		v, e := call(c)
		if e != nil {
			t.Fatalf("%s: %v", c.Action, e)
		}
		return v
	}
	a := WorkspaceIdentity{"client-a", "enrollment-a"}
	b := WorkspaceIdentity{"client-b", "enrollment-b"}
	p := Policy{Execution: 3, Validation: 2, BacklogHigh: 8, BacklogLow: 0, BacklogHard: 9, AcceptanceHigh: 8, AcceptanceLow: 0, WorkspaceQuotas: []WorkspaceQuota{{a, 3, 1, 1}, {b, 3, 1, 1}}}
	must(Command{Action: "policy", Policy: p, Reason: "bounded client admission"})
	for _, x := range []struct {
		o string
		w WorkspaceIdentity
	}{{"a1", a}, {"a2", a}, {"b1", b}} {
		must(Command{Action: "commit", Order: x.o, Obligation: x.o, Scope: "scope", Owner: "worker", Workspace: x.w})
	}
	reserve := func(o string, w WorkspaceIdentity) Command {
		return Command{Action: "start", Order: o, Obligation: o, Reservation: Reservation{ID: o, Attempt: o, Intent: o, Executor: "guest", Units: 1, Workspace: w}}
	}
	if v := must(reserve("a1", a)); v.Version != WorkspaceVersion {
		t.Fatal("missing fail-closed version marker", v.Version)
	}
	must(Command{Action: "unknown", Order: "a1", Obligation: "a1", Reservation: Reservation{ID: "a1"}, Reason: "lost ACK", Evidence: "native-query-unknown"})
	s = Store{Dir: dir, Gate: s.Gate} // restart must retain unknown charge
	if _, e := call(reserve("a2", a)); !errors.Is(e, ErrHeld) {
		t.Fatalf("unknown released client quota: %v", e)
	}
	independent := reserve("b1", b)
	independent.Bypasses = []Bypass{{Order: "a2", Reason: "client-a quota held", Owner: "admission", Release: "settle a1", Evidence: "canonical reservation a1 unknown; independent client"}}
	must(independent)
	bad := p
	bad.WorkspaceQuotas = append([]WorkspaceQuota{}, p.WorkspaceQuotas...)
	bad.WorkspaceQuotas[0].Workspace.Incarnation = "new-enrollment"
	if _, e := call(Command{Action: "policy", Policy: bad, Reason: "replace client"}); !errors.Is(e, ErrHeld) {
		t.Fatalf("changed active incarnation: %v", e)
	}
	if _, e := call(Command{Action: "commit", Order: "unscoped", Obligation: "u", Owner: "w", Scope: "s"}); !errors.Is(e, ErrHeld) {
		t.Fatalf("missing workspace admitted: %v", e)
	}
	must(Command{Action: "settle", Order: "a1", Obligation: "a1", Reservation: Reservation{ID: "a1"}, Reason: "terminal original native result", Evidence: "exact terminal receipt"})
	must(reserve("a2", a))
	wrong := ReservationCheck{Order: "a2", Obligation: "a2", Scope: "scope", Attempt: "a2", Intent: "a2", Executor: "guest", PolicyRevision: 1, Workspace: b}
	called := false
	if e := s.WithReservationGuard(ctx, "a2", wrong, func(context.Context) error { called = true; return nil }); !errors.Is(e, ErrHeld) || called {
		t.Fatalf("cross-client guard: %v called=%v", e, called)
	}
}

func TestWorkspaceQuotasBoundCommitmentsAndValidatePolicy(t *testing.T) {
	w := WorkspaceIdentity{"client", "incarnation"}
	p := Policy{Execution: 2, Validation: 1, WorkspaceQuotas: []WorkspaceQuota{{w, 1, 1, 1}}}
	if !validWorkspacePolicy(p) {
		t.Fatal("valid quota rejected")
	}
	p.WorkspaceQuotas = append(p.WorkspaceQuotas, p.WorkspaceQuotas[0])
	if validWorkspacePolicy(p) {
		t.Fatal("duplicate client accepted")
	}
	s := initial()
	s.Policy = p
	s.Policy.WorkspaceQuotas = s.Policy.WorkspaceQuotas[:1]
	s.Entries["o"] = Entry{Workspace: w, Phase: "Acceptance"}
	if e := checkWorkspaceCommit(s, w); !errors.Is(e, ErrHeld) {
		t.Fatal(e)
	}
	s.Entries["o"] = Entry{Workspace: w, Phase: "Delivered"}
	if e := checkWorkspaceCommit(s, w); e != nil {
		t.Fatal(e)
	}
}
