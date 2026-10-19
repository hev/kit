package nativehost

import (
	"errors"
	"testing"
)

func TestServiceLifetimeRetainsPartialEntryAndCleanupFailure(t *testing.T) {
	enterErr := errors.New("partial entry")
	closeErr := errors.New("cleanup unknown")
	runErr := errors.New("listener failed")
	for _, tc := range []struct {
		name               string
		enter, body, close error
		nilClose           bool
	}{
		{name: "partial entry", enter: enterErr, close: closeErr},
		{name: "successful body", close: closeErr},
		{name: "body and cleanup failed", body: runErr, close: closeErr},
		{name: "missing cleanup", nilClose: true},
		{name: "entry failed without cleanup", enter: enterErr, nilClose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, runs := 0, 0
			var closeFn func() error
			if !tc.nilClose {
				closeFn = func() error { calls++; return tc.close }
			}
			err := withServiceLifetime(closeFn, tc.enter, func() error { runs++; return tc.body })
			if (tc.nilClose && calls != 0) || (!tc.nilClose && calls != 1) {
				t.Fatalf("cleanup calls %d", calls)
			}
			wantRuns := 1
			if tc.enter != nil || tc.nilClose {
				wantRuns = 0
			}
			if runs != wantRuns {
				t.Fatalf("body calls %d", runs)
			}
			for _, want := range []error{tc.enter, tc.body, tc.close} {
				if want != nil && !errors.Is(err, want) {
					t.Fatalf("lost error %v: %v", want, err)
				}
			}
			if tc.nilClose && tc.enter == nil && !errors.Is(err, ErrHeld) {
				t.Fatalf("nil cleanup accepted: %v", err)
			}
		})
	}
}
