package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
)

type usageFixture struct {
	caches   []*secrets.Cache
	bridges  []*usageBridge
	services []*cluster.Service
	dirs     []string
	fake     fakeop.Fake
	policy   secrets.Policy
}

func waitUsage(t *testing.T, what string, f func() bool) {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if f() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("deadline: " + what)
		case <-tick.C:
		}
	}
}
func newUsageFixture(t *testing.T, p secrets.Policy) *usageFixture {
	t.Helper()
	f := &usageFixture{fake: fakeop.Install(t), policy: p}
	path, e := exec.LookPath("op")
	if e != nil || path != filepath.Join(f.fake.Bin, "op") {
		t.Fatal("fixture would execute real provider CLI")
	}
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-account-token")
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "absent"))
	for i, name := range []string{"alpha", "bravo", "charlie"} {
		f.dirs = append(f.dirs, t.TempDir())
		f.selectHost(t, i)
		c := secrets.New()
		f.caches = append(f.caches, c)
		seeds := []string{}
		if i > 0 {
			seeds = append(seeds, f.services[0].Status().Bound)
		}
		cfg := &cluster.Config{Bind: "127.0.0.1:0", Name: name, Seeds: seeds, KeyRef: "secret://test"}
		s := cluster.NewService(context.Background(), cfg, func(context.Context, string) ([]byte, time.Time, error) {
			return []byte("0123456789abcdef0123456789abcdef"), time.Time{}, nil
		}, cluster.LocalFacts(name, false, 0), slog.New(slog.NewTextHandler(io.Discard, nil)))
		t.Cleanup(s.Close)
		f.services = append(f.services, s)
		waitUsage(t, "service ready", func() bool { return s.Status().Running && s.Status().CountersReady })
		b := &usageBridge{service: s, node: name}
		f.bridges = append(f.bridges, b)
		c.AttachCluster(b)
		c.PublishCluster()
	}
	for i, c := range f.caches {
		f.selectHost(t, i)
		c.PublishCluster()
	}
	waitUsage(t, "all usage heartbeats", func() bool {
		for _, b := range f.bridges {
			if len(b.Usage("op", time.Now()).Nodes) != 3 || !b.Usage("op", time.Now()).Visible {
				return false
			}
		}
		return true
	})
	return f
}
func (f *usageFixture) selectHost(t *testing.T, i int) {
	t.Helper()
	d := f.dirs[i]
	t.Setenv("HEV_SECRETS_POLICY", filepath.Join(d, "policy.json"))
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(d, "reads.log"))
	t.Setenv(secrets.CooldownEnv, filepath.Join(d, "cooldown.json"))
	p := f.policy
	p.MeterHost = i == 0
	raw, _ := json.Marshal(p)
	if e := os.WriteFile(filepath.Join(d, "policy.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
}
func (f *usageFixture) read(t *testing.T, i int, item string) error {
	t.Helper()
	f.selectHost(t, i)
	_, e := f.caches[i].ResolveAs(context.Background(), "worker", []secrets.Ref{{Name: "TOKEN", URI: "op://example/" + item + "/password", TTL: time.Hour}})
	return e
}
func TestAccountBudgetPartitionRejoin(t *testing.T) {
	f := newUsageFixture(t, secrets.Policy{Daily: 4, Hourly: 4, CallerMinute: 100, AccountDaily: map[string]int{"op": 6}, AccountHourly: map[string]int{"op": 6}})
	for i := 0; i < 6; i++ {
		if e := f.read(t, i%3, fmt.Sprintf("item%d", i)); e != nil {
			t.Fatal(e)
		}
		want := uint64(i + 1)
		waitUsage(t, "reservation replicated", func() bool {
			for _, b := range f.bridges {
				if b.Usage("op", time.Now()).UsedDay != want {
					return false
				}
			}
			return true
		})
	}
	for i := range f.caches {
		e := f.read(t, i, "refused")
		if !secrets.IsBudgetError(e) || !strings.Contains(e.Error(), "account daily") {
			t.Fatalf("node %d: %v", i, e)
		}
		for _, n := range []string{"alpha", "bravo", "charlie"} {
			if !strings.Contains(e.Error(), n) {
				t.Fatal("missing node spending", e)
			}
		}
	}
	if n := f.fake.Count(t); n != 6 {
		t.Fatalf("requests %d", n)
	}
	if e := f.services[2].Leave(); e != nil {
		t.Fatal(e)
	}
	// A partition is silence of fresh ledger heartbeats, regardless of memberlist
	// suspicion latency. Retained peer counts must age instead of disappearing.
	waitUsage(t, "partition fallback", func() bool { return !f.bridges[2].Usage("op", time.Now()).Visible })
	a := f.bridges[2].Usage("op", time.Now())
	if !strings.Contains(a.Note, "cannot see cluster") || a.Nodes["alpha"].AgeSeconds < 5 || a.Nodes["alpha"].Day != 2 {
		t.Fatalf("dishonest partition status: %+v", a)
	}
	for _, item := range []string{"isolated1", "isolated2"} {
		if e := f.read(t, 2, item); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.read(t, 2, "host-refused"); !secrets.IsBudgetError(e) || !strings.Contains(e.Error(), "host daily") {
		t.Fatalf("host fallback: %v", e)
	}
	if e := f.services[2].Join(f.services[0].Status().Bound); e != nil {
		t.Fatal(e)
	}
	for i, c := range f.caches {
		f.selectHost(t, i)
		c.PublishCluster()
	}
	waitUsage(t, "rejoin converges", func() bool {
		for _, b := range f.bridges {
			a := b.Usage("op", time.Now())
			if !a.Visible || a.UsedDay != 8 {
				return false
			}
		}
		return true
	})
	if e := f.read(t, 2, "rejoined"); !strings.Contains(fmt.Sprint(e), "account daily") {
		t.Fatalf("rejoin guard: %v", e)
	}
	// Encoded gossip facts must never contain the fake credential, value or URI.
	for _, s := range f.services {
		raw := string(s.Status().Counters)
		for _, secret := range []string{fakeop.Value, "fake-account-token", "op://", "password", "TOKEN"} {
			if strings.Contains(raw, secret) {
				t.Fatalf("secret in encoded facts: %s", secret)
			}
		}
	}
}
func TestSharedCooldownAndMeter(t *testing.T) {
	f := newUsageFixture(t, secrets.Policy{Daily: 30, Hourly: 30, CallerMinute: 30, AccountDaily: map[string]int{"op": 60}})
	f.selectHost(t, 0)
	s := f.caches[0].Status(context.Background())
	if len(s.Quota) == 0 {
		t.Fatal("fake meter absent")
	}
	waitUsage(t, "meter shared", func() bool {
		for _, b := range f.bridges {
			if b.SharedMeter().Node != "alpha" {
				return false
			}
		}
		return true
	})
	before := f.fake.Count(t)
	for i, c := range f.caches {
		f.selectHost(t, i)
		s := c.LocalStatus()
		a := s.Accounts[0]
		if a.Meter == nil || a.Meter.Node != "alpha" || a.MeterAgeSeconds < 0 || a.Unexplained != 1000-a.UsedDay || len(s.Quota) == 0 {
			t.Fatalf("shared meter: %+v", s)
		}
	}
	if f.fake.Count(t) != before {
		t.Fatal("status spent a provider request")
	}
	reset := time.Now().Add(2 * time.Second).UTC().Truncate(time.Second)
	t.Setenv("FAKE_OP_RATE_LIMIT", reset.Format(time.RFC3339))
	limitErr := f.read(t, 0, "limited")
	if !secrets.IsRateLimit(limitErr) {
		t.Fatal(limitErr)
	}
	reset = secrets.CooldownReset(limitErr) // includes the existing randomized provider jitter
	t.Setenv("FAKE_OP_RATE_LIMIT", "")
	waitUsage(t, "cooldown shared", func() bool { r, n := f.bridges[1].PeerCooldown("op"); return r.Equal(reset) && n == "alpha" })
	before = f.fake.Count(t)
	for _, i := range []int{1, 2} {
		if e := f.read(t, i, "peer-refused"); !secrets.IsRateLimit(e) || !strings.Contains(e.Error(), "learned from peer alpha") {
			t.Fatalf("peer cooldown: %v", e)
		}
		s := f.caches[i].LocalStatus()
		if !strings.Contains(s.LastLimit, "learned from peer") {
			t.Fatal(s.LastLimit)
		}
	}
	if f.fake.Count(t) != before {
		t.Fatal("peer sent request during cooldown")
	}
	timer := time.NewTimer(time.Until(reset))
	<-timer.C
	if e := f.read(t, 1, "after-reset"); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(f.dirs[1], "reads.log"))
	if e != nil || !strings.Contains(string(raw), "kind=cooldown") {
		t.Fatalf("peer cooldown missing from log: %s %v", raw, e)
	}
}

func TestAccountWarningOnceOnMeterHost(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events")
	notify := filepath.Join(dir, "notify")
	if e := os.WriteFile(notify, []byte("#!/bin/sh\nprintf '%s\\n' \"$2\" >> \"$1\"\n"), 0700); e != nil {
		t.Fatal(e)
	}
	f := newUsageFixture(t, secrets.Policy{Daily: 30, Hourly: 30, CallerMinute: 30, AccountDaily: map[string]int{"op": 6}, Thresholds: []int{50}, Notify: []string{notify, events}})
	for j, i := range []int{1, 2, 0, 1, 2, 0} {
		if e := f.read(t, i, fmt.Sprintf("warn%d", j)); e != nil {
			t.Fatal(e)
		}
		want := uint64(j + 1)
		waitUsage(t, "warning totals", func() bool {
			for _, b := range f.bridges {
				if b.Usage("op", time.Now()).UsedDay != want {
					return false
				}
			}
			return true
		})
	}
	raw, e := os.ReadFile(events)
	if e != nil {
		t.Fatal(e)
	}
	if string(raw) != "account-op-day-50\n" {
		t.Fatalf("account warnings should run once: %q", raw)
	}
}

func TestAccountHourlyBudgetAndPolicyMismatch(t *testing.T) {
	f := newUsageFixture(t, secrets.Policy{Daily: 30, Hourly: 30, CallerMinute: 30, AccountDaily: map[string]int{"op": 60}, AccountHourly: map[string]int{"op": 3}})
	for i := 0; i < 3; i++ {
		if e := f.read(t, i, "hourly"); e != nil {
			t.Fatal(e)
		}
		want := uint64(i + 1)
		waitUsage(t, "hourly counts", func() bool {
			for _, b := range f.bridges {
				if b.Usage("op", time.Now()).UsedHour != want {
					return false
				}
			}
			return true
		})
	}
	for i := 0; i < 3; i++ {
		if e := f.read(t, i, "hourly-refused"); !strings.Contains(fmt.Sprint(e), "account hourly") {
			t.Fatalf("node %d: %v", i, e)
		}
	}
	f.policy.AccountHourly = map[string]int{"op": 4}
	f.selectHost(t, 0)
	f.caches[0].PublishCluster()
	waitUsage(t, "policy mismatch visible", func() bool { return strings.Contains(f.bridges[1].Usage("op", time.Now()).Note, "policy mismatch") })
}

func TestAccountGossipLagOvershootBound(t *testing.T) {
	f := newUsageFixture(t, secrets.Policy{Daily: 30, Hourly: 30, CallerMinute: 30, AccountDaily: map[string]int{"op": 1}})
	// One admitted batch per host before deliberately waiting for replication:
	// three hosts can spend at most three, an overshoot of two over budget one.
	for i := 0; i < 3; i++ {
		if e := f.read(t, i, "burst"); e != nil && !secrets.IsBudgetError(e) {
			t.Fatal(e)
		}
	}
	requests := f.fake.Count(t)
	if requests < 1 || requests > 3 {
		t.Fatalf("overshoot exceeds sum of in-flight batches: %d", requests)
	}
	waitUsage(t, "burst converges", func() bool {
		for _, b := range f.bridges {
			if b.Usage("op", time.Now()).UsedDay != uint64(requests) {
				return false
			}
		}
		return true
	})
	for i := 0; i < 3; i++ {
		if e := f.read(t, i, "after-burst"); !strings.Contains(fmt.Sprint(e), "account daily") {
			t.Fatalf("node %d: %v", i, e)
		}
	}
	if f.fake.Count(t) != requests {
		t.Fatal("spent past converged account budget")
	}
	t.Logf("three-host burst: budget=1 requests=%d overshoot=%d bound=2", requests, requests-1)
}
