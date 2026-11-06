package maintenance

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func topologySetup(t *testing.T) (Store, Snapshot) {
	t.Helper()
	s, v := setup(t)
	r := request(v, "capacity-a", "a")
	v, e := s.ObserveCapacity(ctx, r, Capacity{Incarnation: "boot-1", Known: true, Evidence: "capacity/parent", VerifiedAt: time.Now(), Allocatable: Resources{CPUMilli: 4000, MemoryBytes: 4096}})
	must(t, e)
	return s, v
}
func enrollGuest(t *testing.T, s Store, v Snapshot, id, parent string) (Snapshot, error) {
	t.Helper()
	return s.Enroll(ctx, request(v, "enroll-"+id, id), Node{UID: id, Incarnation: "boot-1", Parent: &ParentEnrollment{UID: parent, Incarnation: "boot-1", WorkspaceRef: "workspace/" + id, Allocation: Resources{CPUMilli: 1000, MemoryBytes: 1024}}})
}
func openGuest(t *testing.T, s Store, v Snapshot, id string) Snapshot {
	t.Helper()
	r := request(v, "open-"+id, id)
	r.Health = &Health{Incarnation: "boot-1", Healthy: true, Evidence: "health/" + id, VerifiedAt: time.Now()}
	v, e := s.Uncordon(ctx, r)
	must(t, e)
	return v
}
func TestGuestEnrollmentRequiresKnownParentCapacityAndImmutableTopology(t *testing.T) {
	s, v := setup(t)
	_, e := enrollGuest(t, s, v, "g", "missing")
	if !errors.Is(e, ErrHeld) {
		t.Fatal(e)
	}
	_, e = enrollGuest(t, s, v, "g", "a")
	if !errors.Is(e, ErrHeld) {
		t.Fatal("unknown capacity admitted", e)
	}
	v, e = s.ObserveCapacity(ctx, request(v, "capacity", "a"), Capacity{Incarnation: "boot-1", Known: true, Evidence: "capacity", VerifiedAt: time.Now(), Allocatable: Resources{CPUMilli: 1000, MemoryBytes: 1024}})
	must(t, e)
	v, e = enrollGuest(t, s, v, "g", "a")
	must(t, e)
	_, e = enrollGuest(t, s, v, "g2", "a")
	if !errors.Is(e, ErrHeld) {
		t.Fatal("overcommit", e)
	}
	// Cordon does not release the existing guest's physical allocation.
	v, e = s.Cordon(ctx, request(v, "cordon-g", "g"))
	must(t, e)
	_, e = enrollGuest(t, s, v, "g2", "a")
	if !errors.Is(e, ErrHeld) {
		t.Fatal("cordoned allocation freed", e)
	}
	_, e = s.Enroll(ctx, request(v, "rewrite-g", "g"), Node{UID: "g", Incarnation: "boot-2"})
	if !errors.Is(e, ErrHeld) {
		t.Fatal("rewrote guest incarnation", e)
	}
}
func TestParentCordonFencesGuestsAndUncordonDoesNotResumeThem(t *testing.T) {
	s, v := topologySetup(t)
	v, e := enrollGuest(t, s, v, "g", "a")
	must(t, e)
	v = openGuest(t, s, v, "g")
	v, start := reserve(t, s, v, "guest-attempt", "g")
	before := v.Nodes["g"].AdmissionGeneration
	staleHealth := time.Now()
	v, e = s.Cordon(ctx, request(v, "parent-cordon", "a"))
	must(t, e)
	if !v.Nodes["g"].Cordoned || v.Nodes["g"].AdmissionGeneration <= before {
		t.Fatal("descendant not fenced")
	}
	if !reflect.DeepEqual(v.Operations["parent-cordon"].AffectedNodes, []string{"a", "g"}) {
		t.Fatal(v.Operations["parent-cordon"])
	}
	start.ExpectedRevision = v.Revision
	called := false
	_, e = s.WithStartGuard(ctx, start, func(context.Context) error { called = true; return nil })
	if !errors.Is(e, ErrHeld) || called {
		t.Fatal("guest started after parent barrier", e)
	}
	r := request(v, "parent-drain", "a")
	r.Deadline = time.Now().Add(time.Minute)
	v, e = s.Drain(ctx, r)
	must(t, e)
	if v.Reservations["guest-attempt"].State != "Revoked" {
		t.Fatal("reserved guest not revoked")
	}
	r = request(v, "parent-open", "a")
	r.Health = &Health{Incarnation: "boot-1", Healthy: true, Evidence: "fresh-parent", VerifiedAt: time.Now()}
	v, e = s.Uncordon(ctx, r)
	must(t, e)
	if !v.Nodes["g"].Cordoned {
		t.Fatal("parent reopened guest")
	}
	r = request(v, "guest-open", "g")
	r.Health = &Health{Incarnation: "boot-1", Healthy: true, Evidence: "old-guest", VerifiedAt: staleHealth}
	_, e = s.Uncordon(ctx, r)
	if !errors.Is(e, ErrHeld) {
		t.Fatal("pre-barrier guest health accepted", e)
	}
	r.Health.VerifiedAt = time.Now()
	v, e = s.Uncordon(ctx, r)
	must(t, e)
	must(t, s.CheckAdmission(ctx, "g", "boot-1"))
	// The original reservation remains revoked even after explicit guest recovery.
	start.ExpectedRevision = v.Revision
	_, e = s.WithStartGuard(ctx, start, func(context.Context) error { t.Fatal("old reservation replayed"); return nil })
	if !errors.Is(e, ErrHeld) {
		t.Fatal(e)
	}
}
func TestParentDrainPreservesUnknownAcrossRestartAndUnrelatedProgress(t *testing.T) {
	s, v := topologySetup(t)
	v, e := enrollGuest(t, s, v, "g", "a")
	must(t, e)
	v = openGuest(t, s, v, "g")
	v, start := reserve(t, s, v, "guest-attempt", "g")
	v, e = s.WithStartGuard(ctx, start, func(context.Context) error { return errors.New("lost ACK") })
	if e == nil {
		t.Fatal("missing unknown")
	}
	r := request(v, "parent-drain", "a")
	r.Deadline = time.Now().Add(time.Minute)
	v, e = s.Drain(ctx, r)
	must(t, e)
	if v.Nodes["a"].Phase != "Held" || v.Nodes["g"].Phase != "Held" || v.Reservations["guest-attempt"].State != "Unknown" {
		t.Fatal("parent drain discarded unknown", v.Nodes)
	}
	s = Store{Dir: s.Dir}
	v, e = s.ReadSnapshot()
	must(t, e)
	v, start = reserve(t, s, v, "unrelated", "b")
	v, e = s.WithStartGuard(ctx, start, func(context.Context) error { return nil })
	must(t, e)
	r = request(v, "guest-settle", "g")
	r.Observations = []Observation{{ReservationID: "guest-attempt", State: "Settled", Evidence: "native/terminal"}}
	v, e = s.Reconcile(ctx, r)
	must(t, e)
	if v.Nodes["a"].Phase != "Evacuated" || len(v.Nodes["a"].Holds) != 0 {
		t.Fatal("ancestor hold not reconciled", v.Nodes["a"])
	}
}
func TestGuestFinalGateRechecksParentCapacityAndIncarnation(t *testing.T) {
	s, v := topologySetup(t)
	v, e := enrollGuest(t, s, v, "g", "a")
	must(t, e)
	v = openGuest(t, s, v, "g")
	v, start := reserve(t, s, v, "guest-attempt", "g")
	v, e = s.ObserveCapacity(ctx, request(v, "unknown-capacity", "a"), Capacity{Incarnation: "boot-1", Known: false, Evidence: "transport/unknown"})
	must(t, e)
	start.ExpectedRevision = v.Revision
	_, e = s.WithStartGuard(ctx, start, func(context.Context) error { t.Fatal("Unknown capacity started"); return nil })
	if !errors.Is(e, ErrHeld) {
		t.Fatal(e)
	}
	// Model a stale persisted enrollment observation; unrelated b stays usable.
	n := v.Nodes["a"]
	n.Incarnation = "boot-2"
	v.Nodes["a"] = n
	must(t, s.write(v))
	if e = s.CheckAdmission(ctx, "g", "boot-1"); !errors.Is(e, ErrHeld) {
		t.Fatal("stale parent accepted", e)
	}
	v, e = s.Cordon(ctx, request(v, "unrelated-cordon", "b"))
	must(t, e)
}
func TestTopologyRejectsSelfCyclesAndCorruptAncestorCycles(t *testing.T) {
	s, v := topologySetup(t)
	_, e := enrollGuest(t, s, v, "self", "self")
	if !errors.Is(e, ErrConflict) {
		t.Fatal("self-cycle", e)
	}
	v, e = enrollGuest(t, s, v, "g", "a")
	must(t, e)
	n := v.Nodes["a"]
	n.Parent = &ParentEnrollment{UID: "g", Incarnation: "boot-1", WorkspaceRef: "corrupt", Allocation: Resources{CPUMilli: 1, MemoryBytes: 1}}
	v.Nodes["a"] = n
	must(t, s.write(v))
	_, e = enrollGuest(t, s, v, "h", "a")
	if !errors.Is(e, ErrConflict) {
		t.Fatal("cycle accepted", e)
	}
}
func TestSeparateProcessParentCordonSerializesGuestStart(t *testing.T) {
	s, v := topologySetup(t)
	v, e := enrollGuest(t, s, v, "g", "a")
	must(t, e)
	v = openGuest(t, s, v, "g")
	_, start := reserve(t, s, v, "guest-attempt", "g")
	cmd := exec.Command(os.Args[0], "-test.run=^TestMaintenanceProcessHelper$")
	cmd.Env = append(os.Environ(), "MAINTENANCE_HELPER=cordon", "MAINTENANCE_STORE="+s.Dir)
	begun := make(chan struct{})
	done := make(chan error, 1)
	go func() { <-begun; done <- cmd.Run() }()
	_, e = s.WithStartGuard(ctx, start, func(context.Context) error {
		close(begun)
		limit := time.Now().Add(5 * time.Second)
		for {
			if _, e := os.Stat(filepath.Join(s.Dir, "child-ready")); e == nil {
				break
			}
			if time.Now().After(limit) {
				return errors.New("child did not reach barrier")
			}
			time.Sleep(5 * time.Millisecond)
		}
		read, e := s.ReadSnapshot()
		if e != nil {
			return e
		}
		if read.Nodes["a"].Cordoned || read.Nodes["g"].Cordoned {
			return errors.New("barrier crossed active guest launch")
		}
		return nil
	})
	must(t, e)
	must(t, <-done)
	v, e = s.ReadSnapshot()
	must(t, e)
	if !v.Nodes["a"].Cordoned || !v.Nodes["g"].Cordoned || v.Reservations["guest-attempt"].State != "Running" || len(v.Nodes["a"].Holds) == 0 {
		t.Fatal("parent barrier lost descendant owner", v.Nodes)
	}
}

func TestTopologySnapshotRequiresExplicitReaderVersion(t *testing.T) {
	s, v := topologySetup(t)
	if v.Version != Version {
		t.Fatal("capacity-only upgraded legacy store")
	}
	v, e := enrollGuest(t, s, v, "g", "a")
	must(t, e)
	if v.Version != TopologyVersion {
		t.Fatal("topology did not fence old reader")
	}
	v.Version = Version
	must(t, s.write(v))
	if _, e = s.ReadSnapshot(); e == nil {
		t.Fatal("unversioned topology read accepted")
	}
}
