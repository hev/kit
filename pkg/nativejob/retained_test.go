//go:build linux || darwin

package nativejob

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetainedDeadlineChild(t *testing.T) {
	if os.Getenv("NATIVE_RETAINED_DEADLINE_CHILD") == "short" {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}
	if os.Getenv("NATIVE_RETAINED_DEADLINE_CHILD") == "1" {
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
}
func TestRetainedDeadlineKillsOwnedRootAndRetainsResult(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := guardedRequest(root)
	r.Argv = []string{os.Args[0], "-test.run=^TestRetainedDeadlineChild$"}
	r.Environment = []string{"NATIVE_RETAINED_DEADLINE_CHILD=1"}
	r.PayloadDigest = RequestDigest(r)
	j := mustSubmit(t, s, r)
	calls := 0
	var child Birth
	result, err := s.ExecuteRetainedWithGate(context.Background(), j.ID, newTestGate(r), RetainedExecutionOptions{WorkloadTimeout: time.Second}, func(o StartedObservation) {
		calls++
		if o.Child == nil {
			t.Fatal("no kernel birth")
		}
		child = *o.Child
	})
	if err != nil || calls != 1 || result.State != "Completed" || result.Result == nil || result.Result.ExitCode == 0 || result.Result.Evidence != WorkloadDeadlineEvidence || alive(&child) {
		t.Fatal(result, err, calls, child)
	}
	if _, err = s.ExecuteRetainedWithGate(context.Background(), j.ID, newTestGate(r), RetainedExecutionOptions{WorkloadTimeout: time.Second}, nil); err == nil {
		t.Fatal("deadline result replayed")
	}
	events, err := s.Outbox()
	if err != nil || len(events) != 1 || events[0].ResultDigest != result.ResultDigest {
		t.Fatal(events, err)
	}
}
func TestRetainedTimeoutRequiresExplicitBoundBeforeClaim(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := guardedRequest(root)
	j := mustSubmit(t, s, r)
	for _, timeout := range []time.Duration{0, -1, MaxRetainedWorkload + time.Second} {
		if _, err := s.ExecuteRetainedWithGate(context.Background(), j.ID, newTestGate(r), RetainedExecutionOptions{WorkloadTimeout: timeout}, nil); err == nil {
			t.Fatal("invalid deadline")
		}
	}
	got, err := s.Query(j.ID)
	if err != nil || got.State != "Pending" {
		t.Fatal(got, err)
	}
}

func TestClientCancellationDoesNotBecomeWorkloadDeadline(t *testing.T) {
	for _, retained := range []bool{false, true} {
		root := t.TempDir()
		s := testStore(t, filepath.Join(root, "journal"))
		testFence(t, s)
		r := guardedRequest(root)
		r.Argv = []string{os.Args[0], "-test.run=^TestRetainedDeadlineChild$"}
		r.Environment = []string{"NATIVE_RETAINED_DEADLINE_CHILD=short"}
		r.PayloadDigest = RequestDigest(r)
		j := mustSubmit(t, s, r)
		ctx, cancel := context.WithCancel(context.Background())
		observe := func(StartedObservation) { cancel() }
		var result Job
		var err error
		if retained {
			result, err = s.ExecuteRetainedWithGate(ctx, j.ID, newTestGate(r), RetainedExecutionOptions{WorkloadTimeout: 3 * time.Second}, observe)
		} else {
			result, err = s.ExecuteWithGateObserved(ctx, j.ID, newTestGate(r), observe)
		}
		cancel()
		if err != nil || result.State != "Completed" || result.Result.ExitCode != 0 || result.Result.Evidence != "" {
			t.Fatal("client cancellation terminated workload", retained, result, err)
		}
	}
}
