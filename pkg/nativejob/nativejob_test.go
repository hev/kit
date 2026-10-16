//go:build linux || darwin

package nativejob

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var testEnrollment = Enrollment{HostID: "test-host", ExecutorID: "test-executor"}

func testStore(t *testing.T, root string) *Store {
	t.Helper()
	s, err := Open(root, testEnrollment)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func testRequest(root string) Request {
	r := Request{Version: Version, IntentKey: "intent-1", Identity: Identity{OrderUID: "order", ScopeDigest: "scope", PlanDigest: "plan", AuthorityDigest: "authority", ExecutionEpoch: "epoch", WorkflowUID: "workflow", NodeID: "node", StepID: "step", AttemptID: "attempt", CandidateDigest: "candidate", InputDigest: "inputs", EnvironmentDigest: "env", Operation: "exec", HostID: testEnrollment.HostID, ExecutorID: testEnrollment.ExecutorID, ChefID: "chef", Fence: 1}, Argv: []string{"/bin/sh", "-c", "printf 'one\\n' >> effects; printf 'hello\\n'; printf 'error\\n' >&2"}, Directory: root, Environment: []string{}}
	r.PayloadDigest = RequestDigest(r)
	return r
}
func testFence(t *testing.T, s *Store) {
	t.Helper()
	if err := s.AdvanceFence(Fence{OrderUID: "order", ChefID: "chef", Epoch: 1, Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
}
func mustSubmit(t *testing.T, s *Store, r Request) Job {
	t.Helper()
	j, err := s.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func TestRealJobLostACKRestart(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := testRequest(root)
	j := mustSubmit(t, s, r)
	if again := mustSubmit(t, s, r); again.ID != j.ID {
		t.Fatal("duplicate submit changed ID")
	}
	s.Close()
	s = testStore(t, filepath.Join(root, "journal"))
	got, err := s.Query(r.IntentKey)
	if err != nil || got.ID != j.ID {
		t.Fatal(got, err)
	}
	j, err = s.Execute(j.ID)
	if err != nil || j.State != "Completed" || j.Result.ExitCode != 0 {
		t.Fatal(j, err)
	}
	if _, err = s.Execute(j.ID); err == nil {
		t.Fatal("second execution accepted")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "effects")); string(b) != "one\n" {
		t.Fatalf("effect count: %q", b)
	}
	events, err := s.Outbox()
	if err != nil || len(events) != 1 {
		t.Fatal(events, err)
	}
	before := encode(events)
	s.Close()
	s = testStore(t, filepath.Join(root, "journal"))
	events, err = s.Outbox()
	if err != nil || encode(events) != before {
		t.Fatal("outbox changed on restart", err)
	}
	bad := r.Identity
	bad.AttemptID = "other"
	if err = s.Ack(j.ID, bad, j.EventID, j.ResultDigest); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = s.Ack(j.ID, r.Identity, j.EventID, "wrong"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err = s.Ack(j.ID, r.Identity, j.EventID, j.ResultDigest); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s = testStore(t, filepath.Join(root, "journal"))
	events, err = s.Outbox()
	if err != nil || len(events) != 0 {
		t.Fatal(events, err)
	}
	after, err := s.Query(j.ID)
	if err != nil || !after.Acknowledged || after.ResultDigest != j.ResultDigest {
		t.Fatal(after, err)
	}
	for _, a := range j.Result.Artifacts {
		actual, err := fileArtifact(filepath.Join(s.root, j.ID, a.Name), a.Name)
		if err != nil || actual != a {
			t.Fatal("artifact mismatch", a, actual, err)
		}
	}
}
func TestConflictFenceEnrollment(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := testRequest(root)
	j := mustSubmit(t, s, r)
	changed := r
	changed.Argv = []string{"/bin/false"}
	changed.PayloadDigest = RequestDigest(changed)
	if _, err := s.Submit(changed); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	changed = r
	changed.Identity.AttemptID = "changed"
	if _, err := s.Submit(changed); err == nil {
		t.Fatal("digest mismatch accepted")
	}
	if err := s.AdvanceFence(Fence{OrderUID: "order", ChefID: "new-chef", Epoch: 2, Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(r); !errors.Is(err, ErrFence) {
		t.Fatal("stale duplicate admission", err)
	}
	if _, err := s.Execute(j.ID); !errors.Is(err, ErrFence) {
		t.Fatal("stale execute", err)
	}
	if err := s.AdvanceFence(Fence{OrderUID: "order", ChefID: "chef", Epoch: 1, Expires: time.Now().Add(time.Hour)}); !errors.Is(err, ErrFence) {
		t.Fatal(err)
	}
	if err := s.AdvanceFence(Fence{OrderUID: "order", ChefID: "changed", Epoch: 2, Expires: time.Now().Add(time.Hour)}); !errors.Is(err, ErrFence) {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	r.Identity.Fence = 2
	r.Identity.ChefID = "new-chef"
	r.IntentKey = "new"
	r.PayloadDigest = RequestDigest(r)
	if _, err := s.Submit(r); !errors.Is(err, ErrFence) {
		t.Fatal("expired fence", err)
	}
	s.Close()
	if other, err := Open(filepath.Join(root, "journal"), Enrollment{HostID: "other", ExecutorID: "other"}); err == nil {
		other.Close()
		t.Fatal("enrollment changed")
	}
}
func TestConcurrentAdmissionAndExecution(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "journal")
	s := testStore(t, path)
	testFence(t, s)
	second := testStore(t, path)
	r := testRequest(root)
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for _, store := range []*Store{s, second} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			j, err := s.Submit(r)
			if err != nil {
				errs <- err
			} else {
				ids <- j.ID
			}
		}(store)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	one, two := <-ids, <-ids
	if one != two {
		t.Fatal(one, two)
	}
	success := make(chan bool, 2)
	for _, store := range []*Store{s, second} {
		wg.Add(1)
		go func(s *Store) { defer wg.Done(); _, err := s.Execute(one); success <- err == nil }(store)
	}
	wg.Wait()
	a, b := <-success, <-success
	if a == b {
		t.Fatal("exactly one claimant required", a, b)
	}
	if bytes, _ := os.ReadFile(filepath.Join(root, "effects")); string(bytes) != "one\n" {
		t.Fatalf("effect count %q", bytes)
	}
}

// These helpers actually terminate the supervising process at each boundary,
// so restart tests cannot accidentally rely on deferred cleanup or live memory.
func TestCrashHelper(t *testing.T) {
	root := os.Getenv("NATIVE_JOB_CRASH_ROOT")
	if root == "" {
		return
	}
	stage := os.Getenv("NATIVE_JOB_CRASH_STAGE")
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
	if stage == "intent-committed" || stage == "intent-staged" {
		_, err = s.Submit(testRequest(root))
	} else {
		j, e := s.Query("intent-1")
		if e != nil {
			os.Exit(81)
		}
		if stage == "ack-committed" || stage == "ack-staged" {
			err = s.Ack(j.ID, j.Request.Identity, j.EventID, j.ResultDigest)
		} else {
			_, err = s.Execute(j.ID)
		}
	}
	if err != nil {
		os.Exit(82)
	}
	os.Exit(83)
}
func TestCrashBoundaries(t *testing.T) {
	stages := []string{"intent-staged", "intent-committed", "ownership-staged", "ownership-committed", "before-spawn", "spawned", "running-staged", "running-committed", "process-exited", "artifacts-synced", "result-staged", "result-committed", "ack-staged", "ack-committed"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "journal")
			s := testStore(t, path)
			testFence(t, s)
			r := testRequest(root)
			if stage != "intent-committed" && stage != "intent-staged" {
				j := mustSubmit(t, s, r)
				if stage == "ack-committed" || stage == "ack-staged" {
					if _, err := s.Execute(j.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			s.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashHelper$")
			cmd.Env = append(os.Environ(), "NATIVE_JOB_CRASH_ROOT="+root, "NATIVE_JOB_CRASH_STAGE="+stage)
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 77 {
				t.Fatalf("helper did not reach %s: %v", stage, err)
			}
			s = testStore(t, path)
			j, err := s.Query("intent-1")
			if stage == "intent-staged" {
				if err == nil {
					t.Fatal("uncommitted intent survived")
				}
				j = mustSubmit(t, s, r)
				err = nil
			}
			if err != nil {
				t.Fatal(err)
			}
			if stage == "intent-committed" || stage == "intent-staged" || stage == "ownership-staged" {
				if j.State != "Pending" {
					t.Fatal(j)
				}
				if _, err = s.Execute(j.ID); err != nil {
					t.Fatal(err)
				}
			} else if stage == "result-committed" || stage == "ack-committed" || stage == "ack-staged" {
				if j.State != "Completed" {
					t.Fatal(j)
				}
				events, err := s.Outbox()
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if stage == "ack-committed" {
					want = 0
				}
				if len(events) != want {
					t.Fatal(events)
				}
			} else {
				j, err = s.Recover(j.ID)
				if err != nil || j.State != "Uncertain" {
					t.Fatal(j, err)
				}
				if _, err = s.Execute(j.ID); err == nil {
					t.Fatal("uncertain operation replayed")
				}
				// Child may have outlived supervisor at the spawned boundary. Wait for its
				// observable single effect, then retain an explicit inspection assertion.
				if stage == "spawned" || stage == "running-staged" {
					deadline := time.Now().Add(5 * time.Second)
					for time.Now().Before(deadline) {
						if b, _ := os.ReadFile(filepath.Join(root, "effects")); string(b) == "one\n" {
							break
						}
						time.Sleep(time.Millisecond)
					}
				}
				in := Inspection{ID: "inspection-1", Inspector: "independent-inspector", Evidence: "synthetic-host-effect-inspection", Result: Result{ExitCode: 0, Evidence: "observed effect or verified pre-spawn absence"}}
				j, err = s.Reconcile(j.ID, r.Identity, in)
				if err != nil || j.State != "Completed" {
					t.Fatal(j, err)
				}
				if _, err = s.Reconcile(j.ID, r.Identity, in); err != nil {
					t.Fatal("lost reconciliation ACK", err)
				}
				changed := in
				changed.Evidence = "conflicting"
				if _, err = s.Reconcile(j.ID, r.Identity, changed); !errors.Is(err, ErrConflict) {
					t.Fatal(err)
				}
			}
			content, _ := os.ReadFile(filepath.Join(root, "effects"))
			want := "one\n"
			if stage == "ownership-committed" || stage == "before-spawn" {
				want = ""
			}
			if string(content) != want {
				t.Fatalf("effect count after %s: %q want %q", stage, content, want)
			}
		})
	}
}
func TestBirthMismatchAndProtocol(t *testing.T) {
	birth, err := processBirth(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if !alive(&birth) {
		t.Fatal("self not alive")
	}
	changed := birth
	changed.Start += "0"
	if alive(&changed) {
		t.Fatal("PID reuse accepted")
	}
	changed = birth
	changed.BootID = "other-boot"
	if alive(&changed) {
		t.Fatal("boot mismatch accepted")
	}
	for _, input := range []string{`{"version":"native-jobs/v1","action":"outbox","unknown":1}`, `{} {}`, `bad`} {
		if _, err := DecodeCall(strings.NewReader(input)); err == nil {
			t.Fatal(input)
		}
	}
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	for _, call := range []Call{{Version: "v0", Action: "outbox"}, {Version: Version, Action: "query"}, {Version: Version, Action: "outbox", JobID: "unexpected"}, {Version: Version, Action: "submit"}} {
		if _, err := s.Call(call); err == nil {
			t.Fatal(call)
		}
	}
}

func TestLiveReconcileAndFenceTransferBeforeSpawn(t *testing.T) {
	for _, transfer := range []bool{false, true} {
		t.Run(map[bool]string{false: "live-reconcile", true: "fence-transfer"}[transfer], func(t *testing.T) {
			root := t.TempDir()
			s := testStore(t, filepath.Join(root, "journal"))
			testFence(t, s)
			r := testRequest(root)
			j := mustSubmit(t, s, r)
			s.boundary = func(name string) error {
				if name != "ownership-committed" {
					return nil
				}
				if transfer {
					return s.AdvanceFence(Fence{OrderUID: "order", ChefID: "new", Epoch: 2, Expires: time.Now().Add(time.Hour)})
				}
				_, err := s.Reconcile(j.ID, r.Identity, Inspection{ID: "premature", Inspector: "inspector", Evidence: "not actually stopped", Result: Result{ExitCode: 0}})
				if err == nil {
					t.Fatal("live supervisor reconciled")
				}
				return nil
			}
			result, err := s.Execute(j.ID)
			if transfer {
				if !errors.Is(err, ErrFence) {
					t.Fatal(err)
				}
				if _, err = os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
					t.Fatal("stale owner spawned", err)
				}
			} else {
				if err != nil || result.State != "Completed" {
					t.Fatal(result, err)
				}
			}
		})
	}
}
func TestSpawnFailureHeldAndNewAttempt(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := testRequest(root)
	r.Argv = []string{"/nonexistent/executable"}
	r.PayloadDigest = RequestDigest(r)
	j := mustSubmit(t, s, r)
	if _, err := s.Execute(j.ID); err == nil {
		t.Fatal("expected spawn failure")
	}
	after, err := s.Query(j.ID)
	if err != nil || after.State != "Uncertain" {
		t.Fatal(after, err)
	}
	if _, err = s.Execute(j.ID); err == nil {
		t.Fatal("spawn failure replayed")
	}
	r = testRequest(root)
	r.IntentKey = "reviewed-new-attempt"
	r.Identity.AttemptID = "attempt-2"
	r.PayloadDigest = RequestDigest(r)
	newJob := mustSubmit(t, s, r)
	if newJob.ID == j.ID {
		t.Fatal("new attempt reused job")
	}
	if _, err = s.Execute(newJob.ID); err != nil {
		t.Fatal(err)
	}
	old, err := s.Query(j.ID)
	if err != nil || old.State != "Uncertain" {
		t.Fatal("new attempt mutated old job", old, err)
	}
}

func TestStrictProtocolMembersAndBound(t *testing.T) {
	for _, input := range []string{
		`{"VERSION":"native-jobs/v1","ACTION":"outbox"}`,
		`{"version":"native-jobs/v1","action":"outbox","job_id":""}`,
		`{"version":"native-jobs/v1","action":"outbox","request":null}`,
		`{"version":"native-jobs/v1","action":"query","job_id":"one","result_digest":""}`,
		`{"version":"unsupported","version":"native-jobs/v1","action":"outbox"}`,
		`{"version":"native-jobs/v1","action":"query","job_id":"one","job_id":"two"}`,
		`{"version":"native-jobs/v1","action":"fence","fence":{"order_uid":"one","ORDER_UID":"two"}}`,
		`{"version":"native-jobs/v1","action":"fence","fence":{"epoch":1,"epoch":2}}`,
		`{"version":"native-jobs/v1","action":"outbox"}` + strings.Repeat(" ", 1<<20),
	} {
		if _, err := DecodeCall(strings.NewReader(input)); err == nil {
			t.Fatal("ambiguous protocol accepted", input[:min(100, len(input))])
		}
	}
	valid := encode(Call{Version: Version, Action: "fence", Fence: &Fence{OrderUID: "order", ChefID: "chef", Epoch: 1, Expires: time.Now().Add(time.Hour)}})
	if _, err := DecodeCall(strings.NewReader(valid)); err != nil {
		t.Fatal("valid timestamp rejected", err)
	}
}
