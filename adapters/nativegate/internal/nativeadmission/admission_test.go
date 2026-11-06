package nativeadmission_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hev/kit/adapters/nativegate/internal/businessflow"
	"github.com/hev/kit/adapters/nativegate/internal/chef"
	"github.com/hev/kit/adapters/nativegate/internal/maintenance"
	"github.com/hev/kit/adapters/nativegate/internal/nativeadmission"
)

var ctx = context.Background()

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func temp(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	must(t, e)
	return p
}

type source struct {
	authorize func(context.Context, nativeadmission.Check) error
}

func (s source) Current(context.Context, string) (nativeadmission.OrderState, error) {
	return nativeadmission.OrderState{OrderUID: "order", Revision: "rev", Evidence: "canonical/source", Open: true}, nil
}
func (s source) Authorize(c context.Context, v nativeadmission.Check) error {
	if s.authorize != nil {
		return s.authorize(c, v)
	}
	return nil
}

type fixture struct {
	g      *nativeadmission.Gate
	c      nativeadmission.Check
	lease  nativeadmission.Lease
	cs     chef.Store
	bs     businessflow.Store
	ms     maintenance.Store
	config nativeadmission.Config
	spawn  nativeadmission.SpawnIntent
}

func setup(t *testing.T, guest bool, actions ...string) fixture {
	t.Helper()
	cs := chef.Store{Dir: temp(t)}
	l, e := cs.Acquire("owner", time.Minute)
	must(t, e)
	lease := nativeadmission.Lease{Owner: l.Owner, Fence: l.Fence, Expires: l.Expires}
	bs := businessflow.Store{Dir: temp(t), Gate: businessflow.GateFunc(func(context.Context, businessflow.Command, businessflow.Snapshot) error { return nil })}
	w := businessflow.WorkspaceIdentity{UID: "workspace", Incarnation: "ws-boot"}
	p := businessflow.Policy{Execution: 2, Validation: 1, ReworkProtected: 1, BacklogHigh: 3, BacklogLow: 1, BacklogHard: 3, AcceptanceHigh: 2, AcceptanceLow: 0, WorkspaceQuotas: []businessflow.WorkspaceQuota{{Workspace: w, Commitments: 3, Execution: 2, Validation: 1}}}
	rev := uint64(0)
	for _, c := range []businessflow.Command{{Action: "policy", Policy: p, Reason: "explicit test policy"}, {Action: "commit", Order: "order", Obligation: "obligation", Scope: "scope", Owner: "worker", Workspace: w}, {Action: "start", Order: "order", Obligation: "obligation", Candidate: "candidate", Workspace: w, Reservation: businessflow.Reservation{ID: "reservation", Attempt: "attempt", Executor: "node", Intent: "native-original-intent", Units: 1, Workspace: w}}} {
		c.ExpectedVersion = rev
		c.Key = c.Action
		c.Actor = "owner"
		c.Authority = "authenticated-source"
		v, e := bs.Apply(ctx, c)
		must(t, e)
		rev = v.ResourceVersion
	}
	ms := maintenance.Store{Dir: temp(t)}
	mo := maintenance.Owner{ID: "owner", Fence: 1, ExpiresAt: l.Expires}
	m, e := ms.Claim(ctx, 0, mo)
	must(t, e)
	parent := nativeadmission.Parent{}
	if guest {
		r := maintenance.Request{ExpectedRevision: m.Revision, Owner: mo, NodeUID: "parent", Incarnation: "parent-boot"}
		m, e = ms.Enroll(ctx, r, maintenance.Node{UID: "parent", Incarnation: "parent-boot", Capacity: &maintenance.Capacity{Incarnation: "parent-boot", Known: true, VerifiedAt: time.Now(), Evidence: "capacity", Allocatable: maintenance.Resources{CPUMilli: 4000, MemoryBytes: 4096}}})
		must(t, e)
		r.Key = "open-parent"
		r.ExpectedRevision = m.Revision
		r.Health = &maintenance.Health{Incarnation: "parent-boot", Healthy: true, Evidence: "health", VerifiedAt: time.Now()}
		m, e = ms.Uncordon(ctx, r)
		must(t, e)
		parent = nativeadmission.Parent{UID: "parent", Incarnation: "parent-boot", WorkspaceRef: "workspace/node"}
	}
	n := maintenance.Node{UID: "node", Incarnation: "node-boot"}
	if guest {
		n.Parent = &maintenance.ParentEnrollment{UID: parent.UID, Incarnation: parent.Incarnation, WorkspaceRef: parent.WorkspaceRef, Allocation: maintenance.Resources{CPUMilli: 1000, MemoryBytes: 1024}}
	}
	r := maintenance.Request{ExpectedRevision: m.Revision, Owner: mo, NodeUID: "node", Incarnation: "node-boot"}
	m, e = ms.Enroll(ctx, r, n)
	must(t, e)
	r.Key = "open-node"
	r.ExpectedRevision = m.Revision
	r.Health = &maintenance.Health{Incarnation: "node-boot", Healthy: true, Evidence: "health", VerifiedAt: time.Now()}
	m, e = ms.Uncordon(ctx, r)
	must(t, e)
	c := nativeadmission.Check{Version: nativeadmission.Version, JobID: "job-original", IntentKey: "native-original-intent", PayloadDigest: "sha256:original", NativeFenceExpires: l.Expires, Identity: nativeadmission.Identity{OrderUID: "order", ScopeDigest: "scope", PlanDigest: "plan", AuthorityDigest: "authority", ExecutionEpoch: "epoch", WorkflowUID: "workflow", NodeID: "workflow-node", StepID: "step", AttemptID: "attempt", CandidateDigest: "candidate", InputDigest: "inputs", EnvironmentDigest: "environment", Operation: "exec", HostID: "enrolled-host", ExecutorID: "node", ChefID: l.Owner, Fence: l.Fence}, Authority: nativeadmission.StartAuthority{Version: nativeadmission.Version, NodeUID: "node", Incarnation: "node-boot", AdmissionGeneration: m.Nodes["node"].AdmissionGeneration, ReservationID: "reservation", AttemptAuthority: "reviewed-attempt-authority"}, Workspace: nativeadmission.Workspace{UID: w.UID, Incarnation: w.Incarnation, Reference: "workspace/node"}, Parent: parent}
	config := nativeadmission.Config{ChefDir: cs.Dir, BusinessDir: bs.Dir, MaintenanceDir: ms.Dir, MaintenanceOwner: lease, Authority: source{}}
	g, e := nativeadmission.New(config)
	must(t, e)
	action := "admit-workflow"
	if len(actions) > 0 {
		action = actions[0]
	}
	spawn, e := g.DecideSpawn(ctx, lease, c, nativeadmission.Assessment{Evidence: "assessment/independent", Reason: "launch reviewed admitted native step", Action: action})
	must(t, e)
	return fixture{g, c, lease, cs, bs, ms, config, spawn}
}
func started(r nativeadmission.Receipt) nativeadmission.Started {
	return nativeadmission.Started{State: "NativeSupervisorStarted", JobID: r.JobID, PayloadDigest: r.PayloadDigest, DecisionID: r.DecisionID, AuthorityRevision: r.AuthorityRevision, Evidence: "native/receipt-and-running-committed"}
}
func TestSeparateSpawnGuardNeverWaitsOrReplays(t *testing.T) {
	f := setup(t, true)
	calls := 0
	ack, e := f.g.WithStart(ctx, f.spawn, f.c, func(r nativeadmission.Receipt) (nativeadmission.Started, error) {
		calls++
		currentM, err := f.ms.ReadSnapshot()
		must(t, err)
		currentB, err := f.bs.Snapshot(ctx)
		must(t, err)
		want := fmt.Sprintf("%s@%d;%s@%d;business-policy@%d", currentM.Version, currentM.Revision, currentB.Version, currentB.ResourceVersion, currentB.Reservations["reservation"].PolicyRevision)
		if r.AuthorityRevision != want || currentM.Reservations["reservation"].State != "Starting" || currentB.Reservations["reservation"].Status != "Starting" {
			t.Fatal("receipt did not bind actual Starting snapshots", r.AuthorityRevision, want)
		}
		if r.Authority != f.c.Authority || r.JobID != f.c.JobID || r.PayloadDigest != f.c.PayloadDigest {
			t.Fatal("native binding changed")
		}
		return started(r), nil
	})
	must(t, e)
	if ack.State != "NativeSupervisorStarted" || calls != 1 {
		t.Fatal(ack, calls)
	}
	b, e := f.bs.Snapshot(ctx)
	must(t, e)
	if b.Reservations["reservation"].Status != "Started" {
		t.Fatal(b.Reservations)
	}
	m, e := f.ms.ReadSnapshot()
	must(t, e)
	if m.Reservations["reservation"].State != "Running" {
		t.Fatal(m.Reservations)
	}
	// The launch acknowledgement returned before any process completion. Maintenance
	// can now acquire its lock and close the parent while the effect remains owned.
	_, e = f.ms.Cordon(ctx, maintenance.Request{Key: "parent-close", ExpectedRevision: m.Revision, Owner: m.Owner, NodeUID: "parent", Incarnation: "parent-boot"})
	must(t, e)
	_, e = f.g.WithStart(ctx, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		t.Fatal("replayed spawn")
		return nativeadmission.Started{}, nil
	})
	if !errors.Is(e, nativeadmission.ErrHeld) {
		t.Fatal(e)
	}
}
func TestPendingNilOrWrongACKNeverMeansStarted(t *testing.T) {
	for _, state := range []string{"", "Pending", "Completed"} {
		t.Run(state, func(t *testing.T) {
			f := setup(t, false)
			_, e := f.g.WithStart(ctx, f.spawn, f.c, func(r nativeadmission.Receipt) (nativeadmission.Started, error) {
				a := started(r)
				a.State = state
				return a, nil
			})
			if !errors.Is(e, nativeadmission.ErrHeld) {
				t.Fatal("false success", e)
			}
			b, e := f.bs.Snapshot(ctx)
			must(t, e)
			m, e := f.ms.ReadSnapshot()
			must(t, e)
			if b.Reservations["reservation"].Status != "Unknown" || m.Reservations["reservation"].State != "Unknown" {
				t.Fatal("ambiguity freed reservation")
			}
		})
	}
}
func TestOriginalFenceHandoffAndExpiryUnsupportedBeforeLaunch(t *testing.T) {
	f := setup(t, false)
	l := chef.Lease{Owner: f.lease.Owner, Fence: f.lease.Fence, Expires: f.lease.Expires}
	must(t, f.cs.Handoff(l, "retained original pending reservation"))
	_, e := f.cs.Acquire("replacement", time.Minute)
	must(t, e)
	_, e = f.g.WithStart(ctx, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		t.Fatal("old native identity launched")
		return nativeadmission.Started{}, nil
	})
	if !errors.Is(e, nativeadmission.ErrUnsupportedPendingHandoff) {
		t.Fatal(e)
	}
	b, e := f.bs.Snapshot(ctx)
	must(t, e)
	if b.Reservations["reservation"].Status != "Reserved" {
		t.Fatal("handoff consumed original")
	}
	f = setup(t, false)
	f.c.NativeFenceExpires = time.Now().Add(-time.Second)
	_, e = f.g.WithStart(ctx, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		t.Fatal("expired pending launched")
		return nativeadmission.Started{}, nil
	})
	if !errors.Is(e, nativeadmission.ErrUnsupportedPendingHandoff) {
		t.Fatal(e)
	}
}
func TestChangedIdentityOrWorkspaceCannotLaunch(t *testing.T) {
	f := setup(t, true)
	f.c.Identity.InputDigest = "changed"
	_, e := f.g.WithStart(ctx, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		t.Fatal("changed identity launched")
		return nativeadmission.Started{}, nil
	})
	if !errors.Is(e, nativeadmission.ErrBinding) {
		t.Fatal(e)
	}
}
func TestCanceledAfterAuthorityCannotLaunch(t *testing.T) {
	f := setup(t, false)
	c, cancel := context.WithCancel(ctx)
	calls := 0
	f.config.Authority = source{authorize: func(context.Context, nativeadmission.Check) error {
		calls++
		if calls == 2 {
			cancel()
		}
		return nil
	}}
	g, e := nativeadmission.New(f.config)
	must(t, e)
	_, e = g.WithStart(c, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		t.Fatal("canceled launch")
		return nativeadmission.Started{}, nil
	})
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestLostLaunchACKRetainsUnknownAndCannotReplay(t *testing.T) {
	f := setup(t, false)
	effects := 0
	_, e := f.g.WithStart(ctx, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		effects++
		return nativeadmission.Started{}, errors.New("lost ACK after actual effect")
	})
	if e == nil {
		t.Fatal("lost ACK accepted")
	}
	m, e := f.ms.ReadSnapshot()
	must(t, e)
	if m.Reservations["reservation"].State != "Unknown" {
		t.Fatal(m.Reservations)
	}
	_, e = f.g.WithStart(ctx, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		effects++
		return nativeadmission.Started{}, nil
	})
	if e == nil || effects != 1 {
		t.Fatal("replayed", e, effects)
	}
}
func TestActualProcessCrashAfterSpawnRetainsAllJournals(t *testing.T) {
	f := setup(t, true)
	path := filepath.Join(temp(t), "effect")
	raw, e := json.Marshal(struct {
		Config nativeadmission.Config
		Check  nativeadmission.Check
		Spawn  nativeadmission.SpawnIntent
	}{Config: nativeadmission.Config{ChefDir: f.cs.Dir, BusinessDir: f.bs.Dir, MaintenanceDir: f.ms.Dir, MaintenanceOwner: f.lease}, Check: f.c, Spawn: f.spawn})
	must(t, e)
	cmd := exec.Command(os.Args[0], "-test.run=^TestAdmissionProcessHelper$")
	cmd.Env = append(os.Environ(), "ADMISSION_HELPER="+string(raw), "ADMISSION_EFFECT="+path)
	if e = cmd.Run(); e == nil {
		t.Fatal("expected process death")
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("no actual fixture effect")
	}
	b, e := f.bs.Snapshot(ctx)
	must(t, e)
	m, e := f.ms.ReadSnapshot()
	must(t, e)
	s, e := f.cs.ReadStatus()
	must(t, e)
	if b.Reservations["reservation"].Status != "Starting" || m.Reservations["reservation"].State != "Starting" || s.Attention[0].Status != "applying" {
		t.Fatal("crash lost ownership", b.Reservations, m.Reservations, s.Attention)
	}
	_, e = f.g.WithStart(ctx, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		t.Fatal("crash replay")
		return nativeadmission.Started{}, nil
	})
	if e == nil {
		t.Fatal("crash treated as replayable")
	}
}
func TestAdmissionProcessHelper(t *testing.T) {
	raw := os.Getenv("ADMISSION_HELPER")
	if raw == "" {
		return
	}
	var f struct {
		Config nativeadmission.Config
		Check  nativeadmission.Check
		Spawn  nativeadmission.SpawnIntent
	}
	must(t, json.Unmarshal([]byte(raw), &f))
	f.Config.Authority = source{}
	g, e := nativeadmission.New(f.Config)
	must(t, e)
	_, e = g.WithStart(ctx, f.Spawn, f.Check, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		file, e := os.OpenFile(os.Getenv("ADMISSION_EFFECT"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		must(t, e)
		_, e = file.WriteString("one effect")
		must(t, e)
		must(t, file.Sync())
		must(t, file.Close())
		os.Exit(23)
		return nativeadmission.Started{}, nil
	})
	t.Fatal("expected crash", e)
}

func TestValidationCannotConsumeExecutionReservation(t *testing.T) {
	f := setup(t, false, "request-validation")
	_, e := f.g.WithStart(ctx, f.spawn, f.c, func(nativeadmission.Receipt) (nativeadmission.Started, error) {
		t.Fatal("validation used execution capacity")
		return nativeadmission.Started{}, nil
	})
	if !errors.Is(e, nativeadmission.ErrBinding) {
		t.Fatal(e)
	}
}
