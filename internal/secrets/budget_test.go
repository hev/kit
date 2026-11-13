package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func setPolicy(t *testing.T, p Policy) {
	t.Helper()
	b, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(policyPath(), b, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestHostBudgetSurvivesRestartAndServesHits(t *testing.T) {
	for _, hourly := range []bool{false, true} {
		t.Run(fmt.Sprint(hourly), func(t *testing.T) {
			c := fake(t)
			calls := fakeCalls(t)
			p := Policy{Daily: 3, Hourly: 20, CallerMinute: 20}
			if hourly {
				p.Daily, p.Hourly = 20, 3
			}
			setPolicy(t, p)
			ctx := context.Background()
			r := []Ref{{"A", "op://example/first/token", time.Hour}}
			if _, e := c.ResolveAs(ctx, "first", r); e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 2; i++ {
				if _, e := c.ResolveAs(ctx, fmt.Sprint(i), []Ref{{"A", fmt.Sprintf("op://example/item%d/token", i), time.Hour}}); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := c.ResolveAs(ctx, "loop", []Ref{{"A", "op://example/last/token", time.Hour}}); !IsBudgetError(e) {
				t.Fatalf("budget error: %v", e)
			}
			if _, e := c.ResolveAs(ctx, "loop", r); e != nil {
				t.Fatal("cached read", e)
			}
			restarted := New()
			restarted.op = c.op
			if _, e := restarted.ResolveAs(ctx, "loop", r); !IsBudgetError(e) {
				t.Fatalf("restart bypassed: %v", e)
			}
			if len(calls()) != 3 {
				t.Fatalf("provider called %d times", len(calls()))
			}
			if s := restarted.LocalStatus(); s.Budget.UsedDay != 3 || s.Budget.Throttles != 2 {
				t.Fatalf("status: %+v", s.Budget)
			}
			p.Override = "repair-one"
			setPolicy(t, p)
			if _, e := restarted.ResolveAs(ctx, "loop", r); e != nil {
				t.Fatal("one override", e)
			}
			if _, e := restarted.ResolveAs(ctx, "loop", []Ref{{"A", "op://example/fifth/token", time.Hour}}); !IsBudgetError(e) {
				t.Fatalf("override reused: %v", e)
			}
		})
	}
}
func TestCallerBrakeAndConcurrentReservation(t *testing.T) {
	c := fake(t)
	calls := fakeCalls(t)
	setPolicy(t, Policy{Daily: 100, Hourly: 50, CallerMinute: 2})
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		_, e := c.ResolveAs(ctx, "poller", []Ref{{"A", fmt.Sprintf("op://example/item%d/token", i), time.Hour}})
		if (i < 2) != (e == nil) {
			t.Fatalf("read %d: %v", i, e)
		}
	}
	if len(calls()) != 2 {
		t.Fatal("brake failed")
	}
	s := c.LocalStatus()
	if s.Budget.Throttles != 4 || s.Budget.LastCaller != "poller" {
		t.Fatalf("status %+v", s.Budget)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = reserve(time.Now(), "concurrent", "op", 1) }()
	}
	wg.Wait()
	if s := budgetStatus(time.Now()); s.UsedDay != 4 {
		t.Fatalf("concurrent reservations %+v", s)
	}
}
func TestStaleOnlyOnRateLimitAndRefresh(t *testing.T) {
	for _, failure := range []string{"rate", "auth", "missing", "disabled"} {
		t.Run(failure, func(t *testing.T) {
			c := fake(t)
			calls := fakeCalls(t)
			log := ReadLogPath()
			now := time.Now()
			c.now = func() time.Time { return now }
			r := refs()[:1]
			ctx := context.Background()
			if _, e := c.ResolveAs(ctx, "worker", r); e != nil {
				t.Fatal(e)
			}
			now = now.Add(2 * time.Minute)
			if failure == "rate" || failure == "disabled" {
				t.Setenv("HEV_FAKE_ERROR", "daily rate limit; reset at "+now.Add(time.Minute).UTC().Format(time.RFC3339))
			} else {
				t.Setenv("HEV_FAKE_ERROR", failure)
			}
			if failure == "disabled" {
				setPolicy(t, Policy{Daily: 200, Hourly: 40, CallerMinute: 5, DisableStale: true})
			}
			values, sources, e := c.ResolveSources(ctx, "worker", r)
			if failure == "rate" {
				if e != nil || values["TOKEN"] == "" || sources["TOKEN"] != "stale" {
					t.Fatalf("stale %v %v", sources, e)
				}
				if _, _, e = c.ResolveSources(ctx, "worker", r); e != nil {
					t.Fatal(e)
				}
				if len(calls()) != 2 {
					t.Fatal("cooldown requested")
				}
				if c.LocalStatus().Stale != 2 {
					t.Fatal("stale not surfaced")
				}
				b, _ := os.ReadFile(log)
				if !strings.Contains(string(b), "kind=stale") {
					t.Fatal("stale not logged")
				}
				now = now.Add(2 * time.Hour)
				t.Setenv("HEV_FAKE_ERROR", "")
				_, sources, e = c.ResolveSources(ctx, "worker", r)
				if e != nil || sources["TOKEN"] != "read" {
					t.Fatalf("refresh %v %v", sources, e)
				}
			} else {
				if e == nil {
					t.Fatal("served stale after non-rate error or opt out")
				}
				if failure != "disabled" {
					c.mu.Lock()
					_, held := c.entries[r[0].URI]
					c.mu.Unlock()
					if held {
						t.Fatal("held revoked value")
					}
				}
			}
		})
	}
}
func TestWarningsPersistCrossingsAndHostConfigOnly(t *testing.T) {
	c := fake(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "events")
	// The fake helper is this test binary, never a real op or shell.
	exe, _ := os.Executable()
	t.Setenv("HEV_NOTIFY_TEST_OUT", out)
	setPolicy(t, Policy{Daily: 10, Hourly: 10, CallerMinute: 10, Notify: []string{exe, "-test.run=TestNotifyHelper", "--", out}, Thresholds: []int{50, 80}})
	now := time.Now()
	c.now = func() time.Time { return now }
	// Helper selects through argv; credentials are absent from its environment.
	if e := reserve(now, "worker", "op", 5); e != nil {
		t.Fatal(e)
	}
	limit, used := int64(100), int64(50)
	s := Status{Quota: []QuotaRow{{Type: "account", Action: "read_write", Limit: &limit, Used: &used}}}
	c.warnings(context.Background(), s)
	New().warnings(context.Background(), s)
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Count(string(b), "\n") != 3 {
		t.Fatalf("crossings: %s", b)
	}
	used = 80
	if e := reserve(now, "worker", "op", 3); e != nil {
		t.Fatal(e)
	}
	c.warnings(context.Background(), s)
	b, _ = os.ReadFile(out)
	if strings.Count(string(b), "\n") != 6 {
		t.Fatalf("80%% crossings: %s", b)
	}
}
func TestNotifyHelper(t *testing.T) {
	for i, a := range os.Args {
		if a == "--" && i+2 < len(os.Args) {
			if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") != "" {
				os.Exit(2)
			}
			f, e := os.OpenFile(os.Args[i+1], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if e != nil {
				os.Exit(3)
			}
			fmt.Fprintln(f, os.Args[i+2])
			f.Close()
			return
		}
	}
}
func TestBudgetFailsClosedOnCorruption(t *testing.T) {
	fake(t)
	if e := os.WriteFile(filepath.Join(policyDir(), "budget.json"), []byte("bad"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := reserve(time.Now(), "worker", "op", 1); !IsBudgetError(e) {
		t.Fatalf("corruption %v", e)
	}
}

func TestMeterIsBudgetedAndUnavailableReasonIsVisible(t *testing.T) {
	c := fake(t)
	calls := fakeCalls(t)
	setPolicy(t, Policy{Daily: 1, Hourly: 1, CallerMinute: 5})
	t.Setenv("HEV_FAKE_ARRAY", "1")
	s := c.Status(context.Background())
	if len(s.Quota) != 2 || len(calls()) != 1 || s.Budget.UsedDay != 1 || s.QuotaCheckedAt.IsZero() {
		t.Fatalf("meter status %+v calls=%v", s, calls())
	}
	now := time.Now().Add(6 * time.Minute)
	c.now = func() time.Time { return now }
	s = c.Status(context.Background())
	if len(calls()) != 1 || s.QuotaUnavailable != "host budget refused meter check" {
		t.Fatalf("unbudgeted meter %+v", s)
	}
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	s = c.Status(context.Background())
	if s.QuotaUnavailable != "no service-account credential configured" {
		t.Fatalf("missing credential reason: %+v", s)
	}
}

func TestScheduledMeterNotifiesOnce(t *testing.T) {
	c := fake(t)
	calls := fakeCalls(t)
	c.monitorEvery = 5 * time.Millisecond
	t.Setenv("HEV_FAKE_ARRAY", "1")
	out := filepath.Join(t.TempDir(), "scheduled-events")
	exe, _ := os.Executable()
	setPolicy(t, Policy{Daily: 100, Hourly: 100, CallerMinute: 5, Thresholds: []int{50, 80}, Notify: []string{exe, "-test.run=TestNotifyHelper", "--", out}})
	t.Setenv(EndpointEnv, isolatedEndpoint(t))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stop, e := Serve(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			t.Fatal("scheduled notification did not arrive")
		case <-tick.C:
			b, _ := os.ReadFile(out)
			if len(b) > 0 {
				stop()
				if len(calls()) != 1 || strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), "token-read-50") {
					t.Fatalf("scheduled events %s calls %v", b, calls())
				}
				return
			}
		}
	}
}
