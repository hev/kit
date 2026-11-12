package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

// The scheduled update runs between cycles, never during one, and a switch
// stops the loop with ErrRestart so the daemon re-executes onto the new build.
func TestAfterCycleRunsBetweenCyclesAndRestarts(t *testing.T) {
	var inCycle atomic.Bool
	var events []string
	saved := AfterCycle
	t.Cleanup(func() { AfterCycle = saved })
	checks := 0
	AfterCycle = func(context.Context, *slog.Logger) bool {
		if inCycle.Load() {
			t.Error("update check ran during a cycle")
		}
		checks++
		events = append(events, "check")
		return checks == 3
	}
	run := func() {
		inCycle.Store(true)
		events = append(events, "scan")
		time.Sleep(time.Millisecond)
		inCycle.Store(false)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := scanLoop(context.Background(), nil, run, time.Millisecond, logger)
	if !errors.Is(err, ErrRestart) {
		t.Fatalf("err = %v, want ErrRestart", err)
	}
	if want := []string{"scan", "check", "scan", "check", "scan", "check"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v", events)
	}
}

// With no hook (an older wiring, or a test) the loop is unchanged.
func TestNoAfterCycleKeepsLooping(t *testing.T) {
	saved := AfterCycle
	AfterCycle = nil
	t.Cleanup(func() { AfterCycle = saved })
	ctx, cancel := context.WithCancel(context.Background())
	scans := 0
	err := scanLoop(ctx, nil, func() {
		scans++
		if scans == 2 {
			cancel()
		}
	}, time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil || scans != 2 {
		t.Fatalf("err = %v, scans = %d", err, scans)
	}
}
