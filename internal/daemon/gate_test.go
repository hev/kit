package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"
)

// The first scan must not begin before the gateway is healthy: scan_interval
// defaults to five minutes, so a first cycle that loses the race to the
// containers leaves a new user watching nothing happen.
func TestFirstScanWaitsForGatewayHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []string
	probes := 0
	healthy := func() error {
		probes++
		if probes < 3 {
			events = append(events, "unhealthy")
			return errors.New("connection refused")
		}
		events = append(events, "healthy")
		return nil
	}
	gate := func(ctx context.Context) error { return waitHealthy(ctx, healthy, time.Millisecond, time.Second) }
	run := func() {
		events = append(events, "scan")
		cancel()
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := scanLoop(ctx, gate, run, time.Hour, logger); err != nil {
		t.Fatal(err)
	}
	if want := []string{"unhealthy", "unhealthy", "healthy", "scan"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

// A gateway that never comes up delays the first scan; it does not cancel it.
func TestFirstScanRunsAfterGateTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scans := 0
	gate := func(ctx context.Context) error {
		return waitHealthy(ctx, func() error { return errors.New("down") }, time.Millisecond, 20*time.Millisecond)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := scanLoop(ctx, gate, func() { scans++; cancel() }, time.Hour, logger); err != nil {
		t.Fatal(err)
	}
	if scans != 1 {
		t.Fatalf("scans = %d", scans)
	}
}

// A hosted config has no gate, and scans at once as it always has.
func TestUnmanagedDaemonScansImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scans := 0
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := scanLoop(ctx, nil, func() { scans++; cancel() }, time.Hour, logger); err != nil {
		t.Fatal(err)
	}
	if scans != 1 {
		t.Fatalf("scans = %d", scans)
	}
}

// Only a config `hev up` wrote gets the gate, and the gate asks the health
// check it was given.
func TestManagedConfigGetsTheGate(t *testing.T) {
	asked := 0
	healthy := func() error { asked++; return nil }
	if gate := firstScanGate(&Config{}, healthy); gate != nil {
		t.Fatal("an unmanaged config got a gate")
	}
	gate := firstScanGate(&Config{Local: LocalConfig{Managed: true}}, healthy)
	if gate == nil {
		t.Fatal("a managed config got no gate")
	}
	if err := gate(context.Background()); err != nil || asked != 1 {
		t.Fatalf("err=%v asked=%d", err, asked)
	}
}
