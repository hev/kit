//go:build linux || darwin

package nativejob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunningObserverAfterGateReturnAndBeforeWait(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := guardedRequest(root)
	release := filepath.Join(root, "release")
	r.Argv = []string{os.Args[0], "-test.run=^TestObserverChild$"}
	r.Environment = []string{"NATIVE_OBSERVER_RELEASE=" + release}
	r.PayloadDigest = RequestDigest(r)
	j := mustSubmit(t, s, r)
	gateReturned := false
	calls := 0
	underlying := newTestGate(r)
	gate := gateFunc(func(ctx context.Context, c StartCheck, start func(StartReceipt) error) error {
		err := underlying.WithStart(ctx, c, start)
		gateReturned = true
		return err
	})
	result, err := s.ExecuteWithGateObserved(context.Background(), j.ID, gate, func(o StartedObservation) {
		calls++
		if !gateReturned || o.JobID != j.ID || o.PayloadDigest != r.PayloadDigest || o.Owner.PID == 0 || !alive(&o.Owner) || o.Child == nil || !alive(o.Child) || o.StartReceipt == nil {
			t.Fatal("early or unbound observation", o)
		}
		// Observation mutation cannot corrupt the retained receipt/birth or result.
		if err := os.WriteFile(release, []byte("release"), 0600); err != nil {
			t.Fatal(err)
		}
		o.Child.Start = "changed"
		o.StartReceipt.DecisionID = "changed"
	})
	if err != nil || calls != 1 || result.State != "Completed" || result.StartReceipt.DecisionID == "changed" || result.Birth.Start == "changed" {
		t.Fatal(result, err, calls)
	}
	events, err := s.Outbox()
	if err != nil || len(events) != 1 || events[0].StartReceipt.DecisionID == "changed" {
		t.Fatal(events, err)
	}
}

func TestRunningObserverDoesNotPublishFailedGateOrBoundary(t *testing.T) {
	for _, mode := range []string{"before-spawn", "after-start", "running-hook"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			s := testStore(t, filepath.Join(root, "journal"))
			testFence(t, s)
			r := guardedRequest(root)
			j := mustSubmit(t, s, r)
			underlying := newTestGate(r)
			gate := gateFunc(func(ctx context.Context, c StartCheck, start func(StartReceipt) error) error {
				if mode == "before-spawn" {
					return ErrStartGate
				}
				if err := underlying.WithStart(ctx, c, start); err != nil {
					return err
				}
				if mode == "after-start" {
					return errors.New("facade commit acknowledgement lost")
				}
				return nil
			})
			if mode == "running-hook" {
				s.boundary = func(name string) error {
					if name == "running-committed" {
						return errors.New("interruption")
					}
					return nil
				}
			}
			calls := 0
			if _, err := s.ExecuteWithGateObserved(context.Background(), j.ID, gate, func(StartedObservation) { calls++ }); err == nil || calls != 0 {
				t.Fatal("false ready", err, calls)
			}
			if _, err := s.ExecuteWithGateObserved(context.Background(), j.ID, gate, func(StartedObservation) { calls++ }); err == nil || calls != 0 {
				t.Fatal("replayed", err, calls)
			}
		})
	}
}

func TestObserverChild(t *testing.T) {
	release := os.Getenv("NATIVE_OBSERVER_RELEASE")
	if release == "" {
		return
	}
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if _, err := os.Stat(release); err == nil {
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(2)
}
