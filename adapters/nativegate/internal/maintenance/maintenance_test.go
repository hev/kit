package maintenance

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var ctx = context.Background()

// TempDir may use a platform alias such as /var on macOS. Fixtures obey the
// runtime canonical-path contract; explicit alias-negative cases remain below.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return dir
}

func setup(t *testing.T) (Store, Snapshot) {
	t.Helper()
	s := Store{Dir: canonicalTempDir(t)}
	v, e := s.Claim(ctx, 0, Owner{ID: "chef", Fence: 1, ExpiresAt: time.Now().Add(time.Hour)})
	must(t, e)
	for _, id := range []string{"a", "b"} {
		v, e = s.Enroll(ctx, request(v, "enroll-"+id, id), Node{UID: id, Incarnation: "boot-1"})
		must(t, e)
		r := request(v, "open-"+id, id)
		r.Health = &Health{Incarnation: "boot-1", Evidence: "health/" + id, VerifiedAt: time.Now(), Healthy: true}
		v, e = s.Uncordon(ctx, r)
		must(t, e)
	}
	return s, v
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func request(v Snapshot, key, node string) Request {
	return Request{Key: key, ExpectedRevision: v.Revision, Owner: v.Owner, NodeUID: node, Incarnation: "boot-1", Reason: "test"}
}
func reserve(t *testing.T, s Store, v Snapshot, id, node string) (Snapshot, StartRequest) {
	t.Helper()
	r := request(v, "reserve-"+id, node)
	x := Reservation{ID: id, NodeUID: node, Incarnation: "boot-1", OrderUID: "order", ObligationID: "obligation", AttemptID: id, Candidate: "sha256:abc", Scope: "scope", AdmissionGeneration: v.Nodes[node].AdmissionGeneration}
	v, e := s.Reserve(ctx, ReservationRequest{r, x})
	must(t, e)
	return v, StartRequest{request(v, "start-"+id, node), id}
}
func TestCordonDelayedReservationAndIndependentNode(t *testing.T) {
	s, v := setup(t)
	v, start := reserve(t, s, v, "one", "a")
	r := request(v, "cordon", "a")
	v, e := s.Cordon(ctx, r)
	must(t, e)
	same, e := s.Cordon(ctx, r)
	must(t, e)
	if same.Revision != v.Revision {
		t.Fatal("duplicate advanced revision")
	}
	start.Request.ExpectedRevision = v.Revision
	called := false
	_, e = s.WithStartGuard(ctx, start, func(context.Context) error { called = true; return nil })
	if !errors.Is(e, ErrHeld) || called {
		t.Fatal("cordoned start ran", e)
	}
	v, start = reserve(t, s, v, "two", "b")
	_, e = s.WithStartGuard(ctx, start, func(context.Context) error { return nil })
	must(t, e)
}
func TestUnknownDrainRestartAndHealth(t *testing.T) {
	s, v := setup(t)
	v, start := reserve(t, s, v, "one", "a")
	v, e := s.WithStartGuard(ctx, start, func(context.Context) error { return errors.New("lost ack") })
	if e == nil || v.Reservations["one"].State != "Unknown" {
		t.Fatal(e)
	}
	r := request(v, "drain", "a")
	r.Deadline = time.Now().Add(time.Hour)
	v, e = s.Drain(ctx, r)
	must(t, e)
	if v.Nodes["a"].Phase != "Held" {
		t.Fatal("unknown not held")
	}
	reopened := Store{Dir: s.Dir}
	v, e = reopened.ReadSnapshot()
	must(t, e)
	r = request(v, "settle", "a")
	r.Observations = []Observation{{ReservationID: "one", State: "Settled", Evidence: "native/settled", CheckpointDigest: "sha256:checkpoint"}}
	v, e = reopened.Reconcile(ctx, r)
	must(t, e)
	if v.Nodes["a"].Phase != "Evacuated" {
		t.Fatal(v.Nodes["a"])
	}
	r = request(v, "reopen", "a")
	r.Health = &Health{Incarnation: "old-boot", Healthy: true, Evidence: "old", VerifiedAt: time.Now()}
	_, e = s.Uncordon(ctx, r)
	if !errors.Is(e, ErrHeld) {
		t.Fatal("accepted stale health")
	}
	r.Health.Incarnation = "boot-1"
	_, e = s.Uncordon(ctx, r)
	must(t, e)
}
func TestTimeoutNonmovableAndExpiredOwner(t *testing.T) {
	s, v := setup(t)
	v, start := reserve(t, s, v, "one", "a")
	v, e := s.WithStartGuard(ctx, start, func(context.Context) error { return nil })
	must(t, e)
	r := request(v, "drain", "a")
	r.Deadline = time.Now().Add(-time.Second)
	v, e = s.Drain(ctx, r)
	must(t, e)
	if len(v.Nodes["a"].Holds) != 2 {
		t.Fatal(v.Nodes["a"])
	}
	read, e := s.ReadSnapshot()
	must(t, e)
	if len(read.Nodes["a"].Holds) != 2 {
		t.Fatal("lost timeout")
	}
	r = request(v, "bad-cas", "b")
	r.ExpectedRevision--
	_, e = s.Cordon(ctx, r)
	if !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	// Expired owner cannot act; a new monotonic owner can recover same journal.
	v.Owner.ExpiresAt = time.Now().Add(-time.Second)
	must(t, s.write(v))
	r = request(v, "expired", "b")
	_, e = s.Cordon(ctx, r)
	if !errors.Is(e, ErrFenced) {
		t.Fatal(e)
	}
	v, e = s.Claim(ctx, v.Revision, Owner{ID: "replacement", Fence: 2, ExpiresAt: time.Now().Add(time.Hour)})
	must(t, e)
	if v.Reservations["one"].State != "Running" {
		t.Fatal("claim reset original effects")
	}
}
func TestSeparateProcessCordonSerializesStart(t *testing.T) {
	s, v := setup(t)
	v, start := reserve(t, s, v, "one", "a")
	cmd := exec.Command(os.Args[0], "-test.run=^TestMaintenanceProcessHelper$")
	cmd.Env = append(os.Environ(), "MAINTENANCE_HELPER=cordon", "MAINTENANCE_STORE="+s.Dir)
	begun := make(chan struct{})
	done := make(chan error, 1)
	go func() { <-begun; done <- cmd.Run() }()
	v, e := s.WithStartGuard(ctx, start, func(context.Context) error {
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
		time.Sleep(20 * time.Millisecond)
		read, e := s.ReadSnapshot()
		if e != nil {
			return e
		}
		if read.Nodes["a"].Cordoned {
			return errors.New("barrier crossed active start")
		}
		return nil
	})
	must(t, e)
	must(t, <-done)
	v, e = s.ReadSnapshot()
	must(t, e)
	if !v.Nodes["a"].Cordoned || v.Reservations["one"].State != "Running" {
		t.Fatal(v)
	}
}
func TestCrashAtStartBoundaryDoesNotReplay(t *testing.T) {
	s, v := setup(t)
	_, _ = reserve(t, s, v, "one", "a")
	cmd := exec.Command(os.Args[0], "-test.run=^TestMaintenanceProcessHelper$")
	cmd.Env = append(os.Environ(), "MAINTENANCE_HELPER=crash", "MAINTENANCE_STORE="+s.Dir)
	if e := cmd.Run(); e == nil {
		t.Fatal("expected crash")
	}
	v, e := s.ReadSnapshot()
	must(t, e)
	if v.Reservations["one"].State != "Starting" {
		t.Fatal(v)
	}
	_, e = s.WithStartGuard(ctx, StartRequest{request(v, "retry", "a"), "one"}, func(context.Context) error { t.Fatal("replayed uncertain effect"); return nil })
	if !errors.Is(e, ErrHeld) {
		t.Fatal(e)
	}
	r := request(v, "drain", "a")
	r.Deadline = time.Now().Add(time.Hour)
	v, e = s.Drain(ctx, r)
	must(t, e)
	if v.Nodes["a"].Phase != "Held" {
		t.Fatal(v.Nodes["a"])
	}
}
func TestMaintenanceProcessHelper(t *testing.T) {
	mode := os.Getenv("MAINTENANCE_HELPER")
	if mode == "" {
		return
	}
	s := Store{Dir: os.Getenv("MAINTENANCE_STORE")}
	for {
		v, e := s.ReadSnapshot()
		must(t, e)
		if mode == "crash" {
			_, e = s.WithStartGuard(ctx, StartRequest{request(v, "start-one", "a"), "one"}, func(context.Context) error { os.Exit(23); return nil })
			t.Fatal(e)
		}
		must(t, os.WriteFile(filepath.Join(s.Dir, "child-ready"), []byte("ready"), 0600))
		_, e = s.Cordon(ctx, request(v, "child-cordon", "a"))
		if errors.Is(e, ErrConflict) {
			continue
		}
		must(t, e)
		return
	}
}
func TestNoDefaultAndReadOnlyMissing(t *testing.T) {
	_, e := (Store{}).Claim(ctx, 0, Owner{})
	if e == nil {
		t.Fatal("default accepted")
	}
	p := filepath.Join(canonicalTempDir(t), "absent")
	_, e = (Store{Dir: p}).ReadSnapshot()
	if !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("read created state")
	}
}

func TestAliasedMissingStoreCannotInitialize(t *testing.T) {
	base := canonicalTempDir(t)
	real := filepath.Join(base, "real")
	must(t, os.Mkdir(real, 0700))
	alias := filepath.Join(base, "alias")
	must(t, os.Symlink(real, alias))
	s := Store{Dir: filepath.Join(alias, "new")}
	_, e := s.Claim(ctx, 0, Owner{ID: "chef", Fence: 1, ExpiresAt: time.Now().Add(time.Hour)})
	if e == nil {
		t.Fatal("alias accepted")
	}
	if _, e = os.Stat(filepath.Join(real, "new")); !os.IsNotExist(e) {
		t.Fatal("mutation followed aliased parent")
	}
}

func TestExhaustedCountersFailBeforeEffects(t *testing.T) {
	s, v := setup(t)
	v, start := reserve(t, s, v, "one", "a")
	v.Revision = ^uint64(0)
	must(t, s.write(v))
	start.ExpectedRevision = v.Revision
	called := false
	_, e := s.WithStartGuard(ctx, start, func(context.Context) error { called = true; return nil })
	if e == nil || called {
		t.Fatal("exhausted revision invoked effect", e)
	}
	read, e := s.ReadSnapshot()
	must(t, e)
	if read.Revision != ^uint64(0) || read.Reservations["one"].State != "Reserved" {
		t.Fatal("counter wrapped or reservation altered")
	}
	s, v = setup(t)
	n := v.Nodes["a"]
	n.AdmissionGeneration = ^uint64(0)
	v.Nodes["a"] = n
	must(t, s.write(v))
	_, e = s.Cordon(ctx, request(v, "max-cordon", "a"))
	if e == nil {
		t.Fatal("generation overflow cordon")
	}
	r := request(v, "max-drain", "a")
	r.Deadline = time.Now().Add(time.Minute)
	_, e = s.Drain(ctx, r)
	if e == nil {
		t.Fatal("generation overflow drain")
	}
	r = request(v, "max-reserve", "a")
	_, e = s.Reserve(ctx, ReservationRequest{r, Reservation{ID: "max", NodeUID: "a", Incarnation: "boot-1", OrderUID: "o", ObligationID: "ob", AttemptID: "attempt", Candidate: "candidate", Scope: "scope", AdmissionGeneration: ^uint64(0)}})
	if e == nil {
		t.Fatal("exhausted generation admitted placement")
	}
	n.Cordoned = true
	n.Phase = "Cordoned"
	v.Nodes["a"] = n
	must(t, s.write(v))
	r = request(v, "max-open", "a")
	r.Health = &Health{Incarnation: "boot-1", Evidence: "health", Healthy: true, VerifiedAt: time.Now()}
	_, e = s.Uncordon(ctx, r)
	if e == nil {
		t.Fatal("generation overflow reopening")
	}
	read, e = s.ReadSnapshot()
	must(t, e)
	if read.Nodes["a"].AdmissionGeneration != ^uint64(0) {
		t.Fatal("generation wrapped")
	}
}
func TestSlowStartSuccessAfterCancellationIsHeld(t *testing.T) {
	s, v := setup(t)
	_, start := reserve(t, s, v, "one", "a")
	c, cancel := context.WithCancel(ctx)
	v, e := s.WithStartGuard(c, start, func(context.Context) error { cancel(); return nil })
	if !errors.Is(e, context.Canceled) || v.Reservations["one"].State != "Unknown" {
		t.Fatal("canceled callback reported known success", v.Reservations["one"], e)
	}
}

func TestSlowStartSuccessAfterLeaseExpiryIsHeld(t *testing.T) {
	// The test must actually enter the callback before asserting post-callback
	// Unknown. Slow fsync/scheduling can consume the lease while durably publishing
	// Starting; that is a safe pre-effect rejection, not a runtime failure.
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for attempt := 1; attempt <= 3; attempt++ {
		s, v := setup(t)
		v, start := reserve(t, s, v, "one", "a")
		expires := time.Now().Add(time.Second)
		v.Owner.ExpiresAt = expires
		must(t, s.write(v))
		start.Owner = v.Owner
		callbacks := 0
		result, err := s.WithStartGuard(bounded, start, func(c context.Context) error {
			callbacks++
			timer := time.NewTimer(time.Until(expires) + 10*time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-c.Done():
				return c.Err()
			}
		})
		if !errors.Is(err, ErrFenced) {
			t.Fatalf("attempt %d: expected lease expiry, callbacks=%d state=%s err=%v", attempt, callbacks, result.Reservations["one"].State, err)
		}
		persisted, readErr := s.ReadSnapshot()
		must(t, readErr)
		state := persisted.Reservations["one"].State
		if callbacks == 1 {
			if result.Reservations["one"].State != "Unknown" || state != "Unknown" {
				t.Fatalf("entered callback crossed expiry without durable Unknown: result=%s persisted=%s", result.Reservations["one"].State, state)
			}
			return
		}
		if callbacks != 0 || (state != "Reserved" && state != "Starting") || result.Reservations["one"].State != state {
			t.Fatalf("unsafe pre-entry expiry: callbacks=%d result=%s persisted=%s", callbacks, result.Reservations["one"].State, state)
		}
		t.Logf("attempt %d safely fenced before callback (%s); retrying a fresh isolated store to exercise post-callback expiry", attempt, state)
	}
	t.Fatal("post-callback expiry was not exercised within three bounded attempts; pre-entry fences were safe")
}
