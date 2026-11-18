package update

import (
	"context"
	"strings"
	"testing"
	"time"
)

// On 2026-11-17 hevd's hourly auto-update moved Oven to a new build while a
// dispatch run was running there, and moved every host ahead of an orderly
// one-host-at-a-time rollout. The daemon now defers while work runs or a hold
// is set, up to [update] max_defer, then updates anyway and says why.
func TestAutoUpdateDefersForRunningWorkAndHold(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	now := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	u := h.updater(edgeA)
	u.Now = func() time.Time { return now }
	u.Config = Config{Auto: true, Channel: ChannelEdge, Interval: "1h", MaxDefer: "6h"}
	busy := []string{"kit-hooks-steering-20261116"}
	u.Busy = func() ([]string, error) { return busy, nil }
	ctx := context.Background()

	// A running dispatch run defers the check: no request, no switch.
	r := u.Auto(ctx)
	if r.Ran || r.Restart || !strings.Contains(r.Deferred, "kit-hooks-steering-20261116") || h.repo.Requests() != 0 {
		t.Fatalf("auto with a running run = %+v (%d requests)", r, h.repo.Requests())
	}
	st, _ := u.LoadState()
	if !st.DeferredSince.Equal(now) || !st.NextCheck.Equal(now.Add(deferRetry)) || h.running(t) != edgeA {
		t.Fatalf("deferred state = %+v", st)
	}
	t.Logf("deferred: %s", r.Deferred)

	// The run ends, but a hold is set: still deferred, and the deferral
	// keeps its start.
	busy = nil
	hold := h.updater(edgeA)
	hold.Now, hold.By = u.Now, "cli"
	if _, err := hold.SetHold(0, "rollout: one host at a time"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(deferRetry)
	r = u.Auto(ctx)
	if r.Ran || !strings.Contains(r.Deferred, "held until released by cli (rollout: one host at a time)") {
		t.Fatalf("auto with a hold = %+v", r)
	}
	st, _ = u.LoadState()
	if !st.DeferredSince.Equal(now.Add(-deferRetry)) {
		t.Fatalf("deferral restarted: %+v", st)
	}
	deferrals := 0
	for _, e := range st.History {
		if e.Result == "deferred" {
			deferrals++
		}
	}
	if deferrals != 2 {
		t.Fatalf("want one deferred event per reason, got %d: %+v", deferrals, st.History)
	}
	t.Logf("deferred: %s", r.Deferred)

	// Past max_defer it updates anyway and says why.
	now = now.Add(6 * time.Hour)
	r = u.Auto(ctx)
	if !r.Ran || !r.Restart || r.Err != nil || h.running(t) != edgeB || !strings.Contains(r.Forced, "max_defer 6h0m0s; updating anyway") || !strings.Contains(r.Forced, "rollout: one host at a time") {
		t.Fatalf("auto past max_defer = %+v", r)
	}
	if len(r.Report.Steps) == 0 || r.Report.Steps[0] != r.Forced {
		t.Fatalf("the report does not say why: %v", r.Report.Steps)
	}
	st, _ = u.LoadState()
	if !st.DeferredSince.IsZero() || st.DeferReason != "" {
		t.Fatalf("deferral not cleared: %+v", st)
	}
	found := false
	for _, e := range st.History {
		found = found || e.Result == "max deferral reached"
	}
	if !found {
		t.Fatalf("history does not record the forced update: %+v", st.History)
	}
	t.Logf("forced: %s", r.Forced)
}

func TestUpdateHoldExpiresAndReleases(t *testing.T) {
	h := newHarness(t, edgeA)
	now := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	u := h.updater(edgeA)
	u.Now = func() time.Time { return now }
	if hd, err := u.LoadHold(); hd != nil || err != nil {
		t.Fatalf("hold before any: %v %v", hd, err)
	}
	if _, err := u.SetHold(2*time.Hour, "  rolling\nout "); err != nil {
		t.Fatal(err)
	}
	hd, err := u.LoadHold()
	if err != nil || hd == nil || hd.Reason != "rolling out" || !hd.Until.Equal(now.Add(2*time.Hour)) || hd.By != "test" {
		t.Fatalf("hold = %+v, %v", hd, err)
	}
	if d := u.Deferral(); !d.Deferred() || d.Hold == nil {
		t.Fatalf("deferral = %+v", d)
	}
	now = now.Add(2 * time.Hour)
	if hd, _ := u.LoadHold(); hd != nil {
		t.Fatal("an expired hold still applies")
	}
	if _, err := u.SetHold(0, ""); err != nil {
		t.Fatal(err)
	}
	if ok, err := u.ReleaseHold(); !ok || err != nil {
		t.Fatalf("release = %v, %v", ok, err)
	}
	if ok, err := u.ReleaseHold(); ok || err != nil {
		t.Fatalf("second release = %v, %v", ok, err)
	}
	if _, err := u.SetHold(-time.Hour, ""); err == nil {
		t.Fatal("negative hold accepted")
	}
	if _, err := u.SetHold(0, strings.Repeat("x", MaxHoldReason+1)); err == nil {
		t.Fatal("overlong reason accepted")
	}
	// Busy listing that fails defers too: the host may not be idle.
	u.Busy = func() ([]string, error) { return nil, context.DeadlineExceeded }
	if d := u.Deferral(); !d.Deferred() || d.BusyError == "" {
		t.Fatalf("deferral with a failed listing = %+v", d)
	}
}

func TestMaxDeferConfig(t *testing.T) {
	if (Config{}).MaxDeferral() != DefaultMaxDefer || DefaultMaxDefer != 24*time.Hour {
		t.Fatal("max_defer default is not 24h")
	}
	if (Config{MaxDefer: "36h"}).MaxDeferral() != 36*time.Hour {
		t.Fatal("max_defer not read")
	}
	if _, err := (Config{MaxDefer: "30s"}).maxDefer(); err == nil {
		t.Fatal("max_defer under 1m accepted")
	}
}
