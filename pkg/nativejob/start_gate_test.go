//go:build linux || darwin

package nativejob

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type testStartGate struct {
	mu        sync.Mutex
	authority StartAuthority
	cordoned  bool
	starts    int
	expect    Identity
}

func (g *testStartGate) WithStart(ctx context.Context, check StartCheck, start func(StartReceipt) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if g.cordoned || check.Version != StartVersion || check.Request.StartAuthority == nil || *check.Request.StartAuthority != g.authority || check.Request.Identity != g.expect {
		return ErrStartGate
	}
	g.starts++
	return start(StartReceipt{Version: StartVersion, JobID: check.JobID, PayloadDigest: check.Request.PayloadDigest, Authority: g.authority, DecisionID: "decision-one", AuthorityRevision: "node-revision-one"})
}

type gateFunc func(context.Context, StartCheck, func(StartReceipt) error) error

func (f gateFunc) WithStart(c context.Context, s StartCheck, start func(StartReceipt) error) error {
	return f(c, s, start)
}
func guardedRequest(root string) Request {
	r := testRequest(root)
	r.StartAuthority = &StartAuthority{Version: StartVersion, NodeUID: "node-uid", Incarnation: "boot-enrollment-one", AdmissionGeneration: 3, ReservationID: "reservation-one", AttemptAuthority: "reviewed-attempt-digest"}
	r.PayloadDigest = RequestDigest(r)
	return r
}
func newTestGate(r Request) *testStartGate {
	return &testStartGate{authority: *r.StartAuthority, expect: r.Identity}
}
func TestGuardCannotBeBypassedAndCordonAfterPending(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := guardedRequest(root)
	j := mustSubmit(t, s, r)
	if _, err := s.Execute(j.ID); !errors.Is(err, ErrStartGate) {
		t.Fatal("plain execute bypass", err)
	}
	if _, err := s.Call(Call{Version: Version, Action: "execute", JobID: j.ID}); !errors.Is(err, ErrStartGate) {
		t.Fatal("wire execute bypass", err)
	}
	state, err := s.Query(j.ID)
	if err != nil || state.State != "Pending" {
		t.Fatal(state, err)
	}
	// Cordon occurs after Submit, not merely before queue admission.
	gate := newTestGate(r)
	gate.cordoned = true
	if _, err = s.ExecuteWithGate(context.Background(), j.ID, gate); !errors.Is(err, ErrStartGate) {
		t.Fatal(err)
	}
	state, err = s.Query(j.ID)
	if err != nil || state.State != "Uncertain" || gate.starts != 0 {
		t.Fatal(state, err)
	}
	if _, err = os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
		t.Fatal("cordoned node spawned", err)
	}
	gate.cordoned = false
	if _, err = s.ExecuteWithGate(context.Background(), j.ID, gate); err == nil {
		t.Fatal("denied claim silently replayed")
	}
}
func TestManagedGateRejectsUnboundLegacyJob(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	j := mustSubmit(t, s, testRequest(root))
	gate := newTestGate(guardedRequest(root))
	if _, err := s.ExecuteWithGate(context.Background(), j.ID, gate); !errors.Is(err, ErrStartGate) {
		t.Fatal(err)
	}
	if _, err := s.Execute(j.ID); err != nil {
		t.Fatal("legacy lane regressed", err)
	}
}
func TestStartAuthorityEachFieldBound(t *testing.T) {
	mutations := []func(*StartAuthority){func(a *StartAuthority) { a.Version = "other" }, func(a *StartAuthority) { a.NodeUID = "other" }, func(a *StartAuthority) { a.Incarnation = "restored-vm" }, func(a *StartAuthority) { a.AdmissionGeneration++ }, func(a *StartAuthority) { a.ReservationID = "other" }, func(a *StartAuthority) { a.AttemptAuthority = "other" }}
	for n, change := range mutations {
		t.Run(string(rune('a'+n)), func(t *testing.T) {
			root := t.TempDir()
			s := testStore(t, filepath.Join(root, "journal"))
			testFence(t, s)
			r := guardedRequest(root)
			j := mustSubmit(t, s, r)
			gate := newTestGate(r)
			change(&gate.authority)
			if _, err := s.ExecuteWithGate(context.Background(), j.ID, gate); !errors.Is(err, ErrStartGate) {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
				t.Fatal("binding mismatch spawned", err)
			}
		})
	}
}
func TestStartReceiptAndACKSurviveRestart(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "journal")
	s := testStore(t, path)
	testFence(t, s)
	r := guardedRequest(root)
	j := mustSubmit(t, s, r)
	gate := newTestGate(r)
	j, err := s.ExecuteWithGate(context.Background(), j.ID, gate)
	if err != nil || j.State != "Completed" || j.StartReceipt == nil || j.StartReceipt.Authority != *r.StartAuthority || gate.starts != 1 {
		t.Fatal(j, err)
	}
	s.Close()
	s = testStore(t, path)
	events, err := s.Outbox()
	if err != nil || len(events) != 1 || encode(events[0].StartReceipt) != encode(j.StartReceipt) {
		t.Fatal(events, err)
	}
	before := encode(events)
	s.Close()
	s = testStore(t, path)
	events, err = s.Outbox()
	if err != nil || encode(events) != before {
		t.Fatal("lostACK event changed", err)
	}
	for range 2 {
		if err = s.Ack(j.ID, r.Identity, j.EventID, j.ResultDigest); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s = testStore(t, path)
	after, err := s.Query(j.ID)
	if err != nil || !after.Acknowledged || encode(after.StartReceipt) != encode(j.StartReceipt) {
		t.Fatal(after, err)
	}
	if bytes, _ := os.ReadFile(filepath.Join(root, "effects")); string(bytes) != "one\n" {
		t.Fatal("effectcount", string(bytes))
	}
}
func TestCordonSerializesActualSpawnNotWholeProcessWait(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := guardedRequest(root)
	r.Argv = []string{"/bin/sh", "-c", "printf 'one\\n' >> effects; while [ ! -f release ]; do sleep 0.01; done"}
	r.PayloadDigest = RequestDigest(r)
	j := mustSubmit(t, s, r)
	gate := newTestGate(r)
	atReceipt := make(chan struct{})
	continueSpawn := make(chan struct{})
	atWait := make(chan struct{})
	continueWait := make(chan struct{})
	s.boundary = func(name string) error {
		if name == "start-receipt-committed" {
			close(atReceipt)
			<-continueSpawn
		}
		if name == "running-committed" {
			close(atWait)
			<-continueWait
		}
		return nil
	}
	result := make(chan error, 1)
	go func() { _, err := s.ExecuteWithGate(context.Background(), j.ID, gate); result <- err }()
	<-atReceipt
	cordoned := make(chan struct{})
	go func() { gate.mu.Lock(); gate.cordoned = true; gate.mu.Unlock(); close(cordoned) }()
	select {
	case <-cordoned:
		t.Fatal("cordon passed receipt/spawn lock")
	case <-time.After(20 * time.Millisecond):
	}
	close(continueSpawn)
	<-atWait
	select {
	case <-cordoned:
	case <-time.After(5 * time.Second):
		t.Fatal("maintenance lock held across process wait")
	}
	if err := os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	close(continueWait)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
func TestGateCallbackFailuresNeverSpawnTwice(t *testing.T) {
	cases := map[string]func(context.Context, StartCheck, func(StartReceipt) error) error{
		"none": func(_ context.Context, _ StartCheck, _ func(StartReceipt) error) error { return nil },
		"wrong-receipt-swallowed": func(_ context.Context, _ StartCheck, start func(StartReceipt) error) error {
			_ = start(StartReceipt{})
			return nil
		},
		"duplicate-swallowed": func(_ context.Context, c StartCheck, start func(StartReceipt) error) error {
			r := StartReceipt{Version: StartVersion, JobID: c.JobID, PayloadDigest: c.Request.PayloadDigest, Authority: *c.Request.StartAuthority, DecisionID: "decision", AuthorityRevision: "rev"}
			_ = start(r)
			_ = start(r)
			return nil
		},
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			s := testStore(t, filepath.Join(root, "journal"))
			testFence(t, s)
			j := mustSubmit(t, s, guardedRequest(root))
			if _, err := s.ExecuteWithGate(context.Background(), j.ID, gateFunc(fn)); !errors.Is(err, ErrStartGate) {
				t.Fatal("invalid gate accepted", err)
			}
			effect, _ := os.ReadFile(filepath.Join(root, "effects"))
			if len(effect) > len("one\n") {
				t.Fatal("duplicate effect", string(effect))
			}
			if name != "duplicate-swallowed" && len(effect) != 0 {
				t.Fatal("bad receipt spawned")
			}
		})
	}
}
func TestGateCrashHelper(t *testing.T) {
	root := os.Getenv("NATIVE_GATE_CRASH_ROOT")
	if root == "" {
		return
	}
	stage := os.Getenv("NATIVE_GATE_CRASH_STAGE")
	s, err := Open(filepath.Join(root, "journal"), testEnrollment)
	if err != nil {
		os.Exit(80)
	}
	s.boundary = func(name string) error {
		if name == stage {
			os.Exit(77)
		}
		return nil
	}
	j, err := s.Query("intent-1")
	if err != nil {
		os.Exit(81)
	}
	_, err = s.ExecuteWithGate(context.Background(), j.ID, newTestGate(j.Request))
	if err != nil {
		os.Exit(82)
	}
	os.Exit(83)
}
func TestGateCrashReceiptBoundaries(t *testing.T) {
	for _, stage := range []string{"start-receipt-staged", "start-receipt-committed", "spawned", "running-staged", "result-staged", "result-committed"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "journal")
			s := testStore(t, path)
			testFence(t, s)
			r := guardedRequest(root)
			j := mustSubmit(t, s, r)
			s.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestGateCrashHelper$")
			cmd.Env = append(os.Environ(), "NATIVE_GATE_CRASH_ROOT="+root, "NATIVE_GATE_CRASH_STAGE="+stage)
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 77 {
				t.Fatal("crash boundary not reached", err)
			}
			s = testStore(t, path)
			after, err := s.Recover(j.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "start-receipt-staged" {
				if after.StartReceipt != nil {
					t.Fatal("uncommitted receipt survived")
				}
			} else if after.StartReceipt == nil {
				t.Fatal("durable receipt missing")
			}
			want := "Uncertain"
			if stage == "result-committed" {
				want = "Completed"
			}
			if after.State != want {
				t.Fatal(after.State, want)
			}
			if _, err = s.ExecuteWithGate(context.Background(), j.ID, newTestGate(r)); err == nil {
				t.Fatal("crash replayed")
			}
			if stage == "start-receipt-staged" || stage == "start-receipt-committed" {
				if _, err = os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
					t.Fatal("pre-spawn effect")
				}
			} else {
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					if b, _ := os.ReadFile(filepath.Join(root, "effects")); string(b) == "one\n" {
						return
					}
					time.Sleep(time.Millisecond)
				}
				t.Fatal("expected exactly one effect")
			}
		})
	}
}
func TestLegacyDigestAndGuardedCapability(t *testing.T) {
	r := testRequest("/tmp/legacy")
	legacy := struct {
		Version       string   `json:"version"`
		IntentKey     string   `json:"intent_key"`
		PayloadDigest string   `json:"payload_digest"`
		Identity      Identity `json:"identity"`
		Argv          []string `json:"argv"`
		Directory     string   `json:"directory"`
		Environment   []string `json:"environment"`
	}{r.Version, r.IntentKey, "", r.Identity, r.Argv, r.Directory, r.Environment}
	if RequestDigest(r) != Digest(legacy) {
		t.Fatal("legacy digest changed")
	}
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	mustSubmit(t, s, guardedRequest(root))
	var marker string
	if err := s.db.QueryRow(`SELECT value FROM metadata WHERE key='identity'`).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	if marker == Version+":"+Digest(testEnrollment) {
		t.Fatal("legacy reader would accept guarded journal")
	}
}
