package update

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Review of the first fix: deferral was checked once, before the download.
// A run that starts, or a hold set, while the build downloads must still stop
// the switch; the host stays on its build and the check is retried.
func TestAutoUpdateRechecksWorkAndHoldBeforeSwitch(t *testing.T) {
	for _, kind := range []string{"running-run", "hold"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t, edgeA)
			h.repo.publish(edgeA, publishOpt{})
			h.repo.publish(edgeB, publishOpt{})
			now := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
			u := h.updater(edgeA)
			u.Now = func() time.Time { return now }
			u.Config = Config{Auto: true, Channel: ChannelEdge, Interval: "1h", MaxDefer: "6h"}
			var busy []string
			u.Busy = func() ([]string, error) { return busy, nil }
			holder := h.updater(edgeA)
			holder.Now, holder.By = u.Now, "cli"
			smoke := u.Smoke
			arrive := true
			u.Smoke = func(ctx context.Context, bin string) (string, error) {
				// The download is done; the work or hold lands now.
				if arrive {
					if kind == "running-run" {
						busy = []string{"new-live-run"}
					} else if _, err := holder.SetHold(0, "rollout"); err != nil {
						t.Fatal(err)
					}
				}
				return smoke(ctx, bin)
			}
			ctx := context.Background()
			r := u.Auto(ctx)
			if r.Err != nil || r.Restart || r.Report.Changed || r.Forced != "" || r.Deferred == "" || h.running(t) != edgeA {
				t.Fatalf("host moved or did not defer with a %s arriving mid-download: %+v (running %s)", kind, r, h.running(t))
			}
			want := map[string]string{"running-run": "running here: new-live-run", "hold": "held until released by cli (rollout)"}[kind]
			if !strings.Contains(r.Deferred, want) {
				t.Fatalf("deferred = %q, want %q", r.Deferred, want)
			}
			if _, err := os.Stat(SwitchingPath(u.Home)); !os.IsNotExist(err) {
				t.Fatalf("switch marker left behind: %v", err)
			}
			st, _ := u.LoadState()
			if !st.DeferredSince.Equal(now) || !st.NextCheck.Equal(now.Add(deferRetry)) || st.Failures != 0 {
				t.Fatalf("late deferral recorded as %+v", st)
			}
			t.Logf("deferred before switching: %s", r.Deferred)

			// The work ends or the hold is released: the retry switches.
			arrive, busy = false, nil
			if _, err := holder.ReleaseHold(); err != nil {
				t.Fatal(err)
			}
			now = now.Add(deferRetry)
			r = u.Auto(ctx)
			if r.Err != nil || !r.Restart || h.running(t) != edgeB {
				t.Fatalf("retry = %+v (running %s)", r, h.running(t))
			}
		})
	}
}

// Past max_defer the update goes ahead even if work arrives mid-download, as
// it would have had the work been there at the first check.
func TestAutoUpdatePastMaxDeferIgnoresLateWork(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	now := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	u := h.updater(edgeA)
	u.Now = func() time.Time { return now }
	u.Config = Config{Auto: true, Channel: ChannelEdge, Interval: "1h", MaxDefer: "2h"}
	busy := []string{"long-run"}
	u.Busy = func() ([]string, error) { return busy, nil }
	ctx := context.Background()
	if r := u.Auto(ctx); r.Deferred == "" {
		t.Fatalf("first check = %+v", r)
	}
	now = now.Add(3 * time.Hour)
	if r := u.Auto(ctx); !r.Restart || r.Forced == "" || h.running(t) != edgeB {
		t.Fatalf("past max_defer = %+v", r)
	}

	// A deferral that only ever arrives mid-download still ends at max_defer.
	const edgeC = "0.7.0-edge.ccccccc"
	h.repo.publish(edgeC, publishOpt{})
	u = h.updater(edgeB)
	u.Now = func() time.Time { return now }
	u.Config = Config{Auto: true, Channel: ChannelEdge, Interval: "1h", MaxDefer: "2h"}
	busy = nil
	u.Busy = func() ([]string, error) { return busy, nil }
	smoke := u.Smoke
	u.Smoke = func(ctx context.Context, bin string) (string, error) {
		busy = []string{"late-run"}
		return smoke(ctx, bin)
	}
	st, _ := u.LoadState()
	st.NextCheck = time.Time{}
	if err := u.saveState(st); err != nil {
		t.Fatal(err)
	}
	if r := u.Auto(ctx); r.Deferred == "" || h.running(t) != edgeB {
		t.Fatalf("late work did not defer: %+v", r)
	}
	now = now.Add(3 * time.Hour)
	busy = nil
	if r := u.Auto(ctx); !r.Restart || h.running(t) != edgeC {
		t.Fatalf("late deferral past max_defer = %+v (running %s)", r, h.running(t))
	}
}

// The other half of the handshake: work registered after the update's last
// check waits out the switch window, then starts on the new build. A hold
// set in the window is seen by that check.
func TestWorkWaitsOutTheSwitchWindow(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	u := h.updater(edgeA)
	u.Config = Config{Auto: true, Channel: ChannelEdge}
	inWindow := false
	switchHook = func() {
		if _, err := os.Stat(SwitchingPath(u.Home)); err != nil {
			t.Errorf("no switch marker in the switch window: %v", err)
		}
		inWindow = awaitSwitch(u.Home, 100*time.Millisecond, 10*time.Millisecond)
		holder := h.updater(edgeA)
		holder.By = "cli"
		go func() { _, _ = holder.SetHold(0, "set in the window") }()
		// SetHold writes the hold before it waits on the marker.
		deadline := time.Now().Add(5 * time.Second)
		for {
			if hold, _ := holder.LoadHold(); hold != nil || time.Now().After(deadline) {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	t.Cleanup(func() { switchHook = nil })
	r := u.Auto(context.Background())
	if !inWindow {
		t.Fatal("admission did not wait in the switch window")
	}
	if r.Restart || h.running(t) != edgeA || !strings.Contains(r.Deferred, "set in the window") {
		t.Fatalf("hold set in the switch window did not stop the switch: %+v", r)
	}
	if awaitSwitch(u.Home, time.Second, time.Millisecond) {
		t.Fatal("waited with no switch under way")
	}
}

func TestAwaitSwitchWaitsForMarkerAndIgnoresStaleOnes(t *testing.T) {
	home := t.TempDir()
	path := SwitchingPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(100 * time.Millisecond); os.Remove(path) }()
	start := time.Now()
	if !awaitSwitch(home, 10*time.Second, 5*time.Millisecond) || time.Since(start) < 100*time.Millisecond {
		t.Fatal("did not wait for the switch to end")
	}
	// A marker left by an updater that died is not honoured.
	if err := os.WriteFile(path, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-switchStale - time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if awaitSwitch(home, 10*time.Second, 5*time.Millisecond) {
		t.Fatal("waited on a stale marker")
	}
	// The wait is bounded.
	if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	if !awaitSwitch(home, 50*time.Millisecond, 5*time.Millisecond) || time.Since(start) > 5*time.Second {
		t.Fatal("wait not bounded")
	}
}
