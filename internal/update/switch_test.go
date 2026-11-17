package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
			if waited, err := awaitSwitch(u.Home, time.Second, time.Millisecond); waited || err != nil {
				t.Fatalf("switch window left open: waited=%v err=%v", waited, err)
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
		waited, err := awaitSwitch(u.Home, 100*time.Millisecond, 10*time.Millisecond)
		inWindow = waited && errors.Is(err, ErrSwitching)
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
	if waited, err := awaitSwitch(u.Home, time.Second, time.Millisecond); waited || err != nil {
		t.Fatalf("waited with no switch under way: %v", err)
	}
}

func TestAwaitSwitchWaitsForTheLockAndIsBounded(t *testing.T) {
	home := t.TempDir()
	// Before any switch there is no lock file, and waiting creates none.
	if waited, err := awaitSwitch(home, time.Second, time.Millisecond); waited || err != nil {
		t.Fatalf("waited with no switch ever begun: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(SwitchingPath(home))); !os.IsNotExist(err) {
		t.Fatalf("waiting created the update directory: %v", err)
	}
	unlock, _, err := lockSwitch(home, time.Second, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	go func(unlock func()) { time.Sleep(100 * time.Millisecond); unlock() }(unlock)
	start := time.Now()
	waited, err := awaitSwitch(home, 10*time.Second, 5*time.Millisecond)
	if err != nil || !waited || time.Since(start) < 100*time.Millisecond {
		t.Fatalf("did not wait for the switch to end: waited=%v err=%v", waited, err)
	}
	// The lock file stays behind; without a lock on it nothing waits, however
	// old or new it is.
	if _, err := os.Stat(SwitchingPath(home)); err != nil {
		t.Fatal(err)
	}
	if waited, err := awaitSwitch(home, 10*time.Second, 5*time.Millisecond); waited || err != nil {
		t.Fatalf("waited with no switch under way: %v", err)
	}
	// The wait is bounded, and running out of it is an error, not a pass.
	unlock, _, err = lockSwitch(home, time.Second, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	start = time.Now()
	waited, err = awaitSwitch(home, 50*time.Millisecond, 5*time.Millisecond)
	if !waited || !errors.Is(err, ErrSwitching) || time.Since(start) > 5*time.Second {
		t.Fatalf("bounded wait: waited=%v err=%v after %s", waited, err, time.Since(start))
	}
}

// Review of the second fix: admission waited a minute and then started work
// although the updater, stalled after its last check, still owned the switch;
// it then switched under that work. Admission now fails while a live
// updater owns the switch, however long it stalls, and the work never starts.
func TestAdmissionFailsWhileALiveSwitchOutlastsItsWait(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	u := h.updater(edgeA)
	u.Config = Config{Auto: true, Channel: ChannelEdge}
	var mu sync.Mutex
	var work []string
	checked, resume := make(chan struct{}), make(chan struct{})
	calls := 0
	u.Busy = func() ([]string, error) {
		calls++
		if calls == 2 {
			// The last check, inside the switch window, found nothing; the
			// updater now stalls before it switches.
			close(checked)
			<-resume
			return nil, nil
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), work...), nil
	}
	result := make(chan AutoResult, 1)
	go func() { result <- u.Auto(context.Background()) }()
	<-checked
	// Admission registers a run, then waits for the switch. The updater
	// stays alive and stalled through several waits, each longer than the
	// previous: none of them lets the work start.
	mu.Lock()
	work = append(work, "dispatch run kit-late")
	mu.Unlock()
	started := false
	for _, wait := range []time.Duration{50 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond} {
		waited, err := awaitSwitch(u.Home, wait, 5*time.Millisecond)
		if err == nil {
			started = true
			break
		}
		if !waited || !errors.Is(err, ErrSwitching) {
			t.Fatalf("admission wait %s: waited=%v err=%v", wait, waited, err)
		}
	}
	if started {
		t.Fatal("admission let work start while a live updater owned the switch")
	}
	// Admission failed: the run is withdrawn and never started.
	mu.Lock()
	work = nil
	mu.Unlock()
	close(resume)
	r := <-result
	if !r.Restart || h.running(t) != edgeB {
		t.Fatalf("the updater, with no work started, should have switched: %+v", r)
	}
	// The switch is over: the retried admission goes straight through, and
	// the work starts on the new build.
	if waited, err := awaitSwitch(u.Home, time.Second, 5*time.Millisecond); waited || err != nil {
		t.Fatalf("retried admission after the switch: waited=%v err=%v", waited, err)
	}
}

// Ownership of the switch ends with the updater's process, not with elapsed
// time: a lock held by a process that dies is free at once.
func TestSwitchLockIsReleasedWhenTheUpdaterDies(t *testing.T) {
	if os.Getenv("HEV_TEST_HOLD_SWITCH") != "" {
		unlock, _, err := lockSwitch(os.Getenv("HEV_TEST_HOLD_SWITCH"), time.Second, time.Millisecond)
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(2)
		}
		defer unlock()
		fmt.Println("locked")
		time.Sleep(time.Minute)
		return
	}
	home := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSwitchLockIsReleasedWhenTheUpdaterDies$", "-test.count=1")
	cmd.Env = append(os.Environ(), "HEV_TEST_HOLD_SWITCH="+home)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := out.Read(buf)
	if !strings.Contains(string(buf[:n]), "locked") {
		_ = cmd.Process.Kill()
		t.Fatalf("helper did not take the lock: %q", buf[:n])
	}
	if _, err := awaitSwitch(home, 100*time.Millisecond, 5*time.Millisecond); !errors.Is(err, ErrSwitching) {
		_ = cmd.Process.Kill()
		t.Fatalf("a live updater's switch was not honoured: %v", err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if waited, err := awaitSwitch(home, 5*time.Second, 5*time.Millisecond); err != nil {
		t.Fatalf("a dead updater's switch still blocks: waited=%v err=%v", waited, err)
	}
}
