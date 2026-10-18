//go:build windows

package nativejob

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

var windowsProofEnrollment = Enrollment{HostID: "isolated-windows-proof", ExecutorID: "isolated-owner"}

// This is the only test route around the production platform-enable guard. It
// executes the same root/DACL/SQLite/barrier implementation as Open. It cannot
// disable a failed filesystem check or enable production through configuration.
func windowsProofOpen(t *testing.T, root string) *Store {
	t.Helper()
	s, err := openJournal(root, windowsProofEnrollment)
	if err != nil {
		t.Fatal("isolated journal hooks (production remains disabled):", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func windowsProofEnv(extra ...string) []string {
	// Explicit minimal environment; never copy credentials into a proof journal.
	return append([]string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}, extra...)
}
func windowsProofRequest(root string, wait bool) Request {
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	env := windowsProofEnv("NATIVE_WINDOWS_JOB_HELPER=1")
	if wait {
		env = append(env, "NATIVE_WINDOWS_JOB_WAIT=1")
	}
	r := Request{Version: Version, IntentKey: "windows-intent", Identity: Identity{OrderUID: "order", ScopeDigest: "scope", PlanDigest: "plan", AuthorityDigest: "authority", ExecutionEpoch: "epoch", WorkflowUID: "workflow", NodeID: "node", StepID: "step", AttemptID: "attempt", CandidateDigest: "candidate", InputDigest: "inputs", EnvironmentDigest: "env", Operation: "exec", HostID: windowsProofEnrollment.HostID, ExecutorID: windowsProofEnrollment.ExecutorID, ChefID: "chef", Fence: 73}, Argv: []string{exe, "-test.run=^TestWindowsJournalEffectHelper$", "--", root}, Directory: root, Environment: env, StartAuthority: &StartAuthority{Version: StartVersion, NodeUID: "node", Incarnation: "incarnation", AdmissionGeneration: 1, ReservationID: "reservation", AttemptAuthority: "attempt"}}
	r.PayloadDigest = RequestDigest(r)
	return r
}
func windowsProofFence(t *testing.T, s *Store) {
	t.Helper()
	if err := s.AdvanceFence(Fence{OrderUID: "order", ChefID: "chef", Epoch: 73, Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
}

type windowsProofGate struct{ check func(context.Context) error }

func (g windowsProofGate) WithStart(context.Context, StartCheck, func(StartReceipt) error) error {
	return ErrStartGate
}
func (g windowsProofGate) WithSpawn(ctx context.Context, c StartCheck, start func(StartReceipt, func(context.Context) error) error) error {
	return start(StartReceipt{Version: StartVersion, JobID: c.JobID, PayloadDigest: c.Request.PayloadDigest, Authority: *c.Request.StartAuthority, DecisionID: "decision", AuthorityRevision: "revision"}, g.check)
}
func windowsProofSuccessGate() windowsProofGate {
	return windowsProofGate{check: func(ctx context.Context) error { return ctx.Err() }}
}

func TestWindowsJournalEffectHelper(t *testing.T) {
	if os.Getenv("NATIVE_WINDOWS_JOB_HELPER") != "1" {
		return
	}
	root := os.Args[len(os.Args)-1]
	f, err := os.OpenFile(filepath.Join(root, "effects"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("one\n"); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(f.Sync(), f.Close()); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(os.Stdout, "private stdout")
	fmt.Fprintln(os.Stderr, "private stderr")
	if os.Getenv("NATIVE_WINDOWS_JOB_WAIT") == "1" {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(root, "release")); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("owned proof child release timed out")
	}
}

func TestWindowsJournalGateCompletionACKReopen(t *testing.T) {
	root := t.TempDir()
	journal := filepath.Join(root, "journal")
	s := windowsProofOpen(t, journal)
	windowsProofFence(t, s)
	r := windowsProofRequest(root, false)
	j, err := s.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := s.Submit(r); err != nil || duplicate.ID != j.ID {
		t.Fatal(duplicate, err)
	}
	if _, err = s.Execute(j.ID); !errors.Is(err, ErrStartGate) {
		t.Fatal("guard bypass", err)
	}
	s.Close()
	s = windowsProofOpen(t, journal)
	receiptDurable := false
	checked := 0
	s.boundary = func(name string) error {
		if name == "start-receipt-committed" {
			receiptDurable = true
		}
		return nil
	}
	gate := windowsProofGate{check: func(ctx context.Context) error {
		checked++
		if !receiptDurable {
			t.Fatal("final check preceded durable receipt")
		}
		if _, err := os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
			t.Fatal("final check after effect", err)
		}
		return ctx.Err()
	}}
	j, err = s.ExecuteWithGate(context.Background(), j.ID, gate)
	if err != nil || j.State != "Completed" || j.Result == nil || j.Result.ExitCode != 0 || j.StartReceipt == nil || j.Birth == nil || j.Owner == nil || checked != 1 {
		t.Fatal(j, err, checked)
	}
	if j.Request.Identity.Fence != 73 || j.StartReceipt.Authority != *r.StartAuthority {
		t.Fatal("original authority changed", j)
	}
	for _, artifact := range j.Result.Artifacts {
		path := filepath.Join(journal, j.ID, artifact.Name)
		if err = windowsVerifyPrivate(path, true, false); err != nil {
			t.Fatal(err)
		}
		got, err := fileArtifact(path, artifact.Name)
		if err != nil || got != artifact {
			t.Fatal("artifact digest mismatch", got, artifact, err)
		}
	}
	events, err := s.Outbox()
	if err != nil || len(events) != 1 {
		t.Fatal(events, err)
	}
	original := encode(events)
	s.Close()
	s = windowsProofOpen(t, journal)
	events, err = s.Outbox()
	if err != nil || encode(events) != original {
		t.Fatal("result/outbox changed on reopen", events, err)
	}
	if _, err = s.ExecuteWithGate(context.Background(), j.ID, gate); err == nil {
		t.Fatal("completed replay")
	}
	bad := r.Identity
	bad.AttemptID = "different"
	if err = s.Ack(j.ID, bad, j.EventID, j.ResultDigest); !errors.Is(err, ErrConflict) {
		t.Fatal("bad identity accepted", err)
	}
	if err = s.Ack(j.ID, r.Identity, j.EventID, "bad-digest"); !errors.Is(err, ErrConflict) {
		t.Fatal("bad digest accepted", err)
	}
	if err = s.AdvanceFence(Fence{OrderUID: "order", ChefID: "new-chef", Epoch: 74, Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	s.boundary = func(name string) error {
		if name == "ack-committed" {
			return errors.New("lost ACK response")
		}
		return nil
	}
	if err = s.Ack(j.ID, r.Identity, j.EventID, j.ResultDigest); err == nil {
		t.Fatal("ACK loss not injected")
	}
	s.Close()
	s = windowsProofOpen(t, journal)
	if err = s.Ack(j.ID, r.Identity, j.EventID, j.ResultDigest); err != nil {
		t.Fatal("historical ACK retry", err)
	}
	after, err := s.Query(j.ID)
	if err != nil || !after.Acknowledged || after.StartReceipt == nil || encode(after.StartReceipt) != encode(j.StartReceipt) || after.ResultDigest != j.ResultDigest {
		t.Fatal(after, err)
	}
	events, err = s.Outbox()
	if err != nil || len(events) != 0 {
		t.Fatal(events, err)
	}
	windowsProofEffectCount(t, root, 1)
}
func windowsProofEffectCount(t *testing.T, root string, count int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "effects"))
	if count == 0 && os.IsNotExist(err) {
		return
	}
	if err != nil || string(data) != strings.Repeat("one\n", count) {
		t.Fatal("effect count", string(data), err)
	}
}

func TestWindowsJournalFinalCheckLossNoReplay(t *testing.T) {
	root := t.TempDir()
	s := windowsProofOpen(t, filepath.Join(root, "journal"))
	windowsProofFence(t, s)
	r := windowsProofRequest(root, false)
	j, err := s.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	durable := false
	s.boundary = func(name string) error {
		if name == "start-receipt-committed" {
			durable = true
		}
		return nil
	}
	gate := windowsProofGate{check: func(context.Context) error {
		if !durable {
			t.Fatal("receipt not durable")
		}
		return ErrFence
	}}
	if _, err = s.ExecuteWithGate(context.Background(), j.ID, gate); !errors.Is(err, ErrFence) {
		t.Fatal(err)
	}
	got, err := s.Query(j.ID)
	if err != nil || got.State != "Uncertain" || got.StartReceipt == nil || got.Birth != nil {
		t.Fatal(got, err)
	}
	s.Close()
	s = windowsProofOpen(t, filepath.Join(root, "journal"))
	if _, err = s.ExecuteWithGate(context.Background(), j.ID, windowsProofSuccessGate()); err == nil {
		t.Fatal("uncertain replay")
	}
	windowsProofEffectCount(t, root, 0)
}

func TestWindowsJournalCrashHelper(t *testing.T) {
	root := os.Getenv("NATIVE_WINDOWS_CRASH_ROOT")
	if root == "" {
		return
	}
	boundary := os.Getenv("NATIVE_WINDOWS_CRASH_BOUNDARY")
	s := windowsProofOpen(t, filepath.Join(root, "journal"))
	windowsProofFence(t, s)
	r := windowsProofRequest(root, boundary == "running-committed")
	s.boundary = func(name string) error {
		if name == boundary {
			os.Exit(91)
		}
		return nil
	}
	j, err := s.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	j, err = s.ExecuteWithGate(context.Background(), j.ID, windowsProofSuccessGate())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Ack(j.ID, r.Identity, j.EventID, j.ResultDigest); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash boundary not reached", boundary)
}

func TestWindowsJournalProcessCrashReopenNoReplay(t *testing.T) {
	for _, boundary := range []string{"intent-committed", "ownership-committed", "start-receipt-committed", "running-committed", "process-exited", "artifacts-synced", "result-committed", "ack-committed"} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			// Release only this fixture's own child, including on assertion failures.
			defer os.WriteFile(filepath.Join(root, "release"), nil, 0600)
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWindowsJournalCrashHelper$")
			cmd.Env = windowsProofEnv("NATIVE_WINDOWS_CRASH_ROOT="+root, "NATIVE_WINDOWS_CRASH_BOUNDARY="+boundary)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 91 {
				t.Fatalf("boundary %s: %v %s", boundary, err, output)
			}
			s := windowsProofOpen(t, filepath.Join(root, "journal"))
			r := windowsProofRequest(root, boundary == "running-committed")
			j, err := s.Query(r.IntentKey)
			if err != nil {
				t.Fatal(err)
			}
			j, err = s.Recover(j.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := "Uncertain"
			if boundary == "intent-committed" {
				expected = "Pending"
			}
			if boundary == "result-committed" || boundary == "ack-committed" {
				expected = "Completed"
			}
			if j.State != expected {
				t.Fatal(j.State, expected)
			}
			if j.Request.PayloadDigest != r.PayloadDigest || j.Request.Identity.Fence != 73 {
				t.Fatal("original request changed", j)
			}
			if expected != "Pending" {
				if _, err = s.ExecuteWithGate(context.Background(), j.ID, windowsProofSuccessGate()); err == nil {
					t.Fatal("crashed job replayed")
				}
			}
			if boundary == "running-committed" {
				if j.Birth == nil || !alive(j.Birth) || j.StartReceipt == nil {
					t.Fatal("original live child/receipt lost", j)
				}
				h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(j.Birth.PID))
				if err != nil {
					t.Fatal(err)
				}
				defer windows.CloseHandle(h)
				if _, err = s.Reconcile(j.ID, r.Identity, Inspection{ID: "premature", Evidence: "owned fixture", Inspector: "proof", Result: Result{}}); err == nil {
					t.Fatal("reconciled while own child live")
				}
				if err = os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				status, err := windows.WaitForSingleObject(h, 10000)
				if err != nil || status != windows.WAIT_OBJECT_0 {
					t.Fatal("own child did not stop", status, err)
				}
			}
			count := 0
			if boundary == "running-committed" || boundary == "process-exited" || boundary == "artifacts-synced" || expected == "Completed" {
				count = 1
			}
			windowsProofEffectCount(t, root, count)
			events, err := s.Outbox()
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "result-committed" && len(events) != 1 {
				t.Fatal(events)
			}
			if boundary == "ack-committed" && (!j.Acknowledged || len(events) != 0) {
				t.Fatal(j, events)
			}
		})
	}
}
