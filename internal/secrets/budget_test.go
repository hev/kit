package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	for _, neutral := range []bool{false, true} {
		t.Run(fmt.Sprintf("neutral=%v", neutral), func(t *testing.T) { testHostBudgetSurvivesRestartAndServesHits(t, neutral) })
	}
}
func testHostBudgetSurvivesRestartAndServesHits(t *testing.T, neutral bool) {
	for _, hourly := range []bool{false, true} {
		t.Run(fmt.Sprint(hourly), func(t *testing.T) {
			c := fake(t)
			if neutral {
				setManager(t, "[secrets]\nmanager = \"1password\"\nvault = \"example\"\nfield = \"token\"\n")
			}
			calls := fakeCalls(t)
			p := Policy{Daily: 3, Hourly: 20, CallerMinute: 20}
			if hourly {
				p.Daily, p.Hourly = 20, 3
			}
			setPolicy(t, p)
			ctx := context.Background()
			r := []Ref{{"A", guardRef(neutral, "first"), time.Hour}}
			if _, e := c.ResolveAs(ctx, "first", r); e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 2; i++ {
				if _, e := c.ResolveAs(ctx, fmt.Sprint(i), []Ref{{"A", guardRef(neutral, fmt.Sprintf("item%d", i)), time.Hour}}); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := c.ResolveAs(ctx, "loop", []Ref{{"A", guardRef(neutral, "last"), time.Hour}}); !IsBudgetError(e) {
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
			if _, e := restarted.ResolveAs(ctx, "loop", []Ref{{"A", guardRef(neutral, "fifth"), time.Hour}}); !IsBudgetError(e) {
				t.Fatalf("override reused: %v", e)
			}
		})
	}
}
func TestCallerBrakeAndConcurrentReservation(t *testing.T) {
	for _, neutral := range []bool{false, true} {
		t.Run(fmt.Sprintf("neutral=%v", neutral), func(t *testing.T) { testCallerBrakeAndConcurrentReservation(t, neutral) })
	}
}
func testCallerBrakeAndConcurrentReservation(t *testing.T, neutral bool) {
	c := fake(t)
	if neutral {
		setManager(t, "[secrets]\nmanager = \"1password\"\nvault = \"example\"\nfield = \"token\"\n")
	}
	calls := fakeCalls(t)
	setPolicy(t, Policy{Daily: 100, Hourly: 50, CallerMinute: 2})
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		_, e := c.ResolveAs(ctx, "poller", []Ref{{"A", guardRef(neutral, fmt.Sprintf("item%d", i)), time.Hour}})
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
	rows, err := Summary(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Caller != "poller" || rows[0].Provider != "op" || rows[0].Requests != 2 || rows[0].Throttles != 4 {
		t.Fatalf("attribution: %+v", rows)
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
	for _, neutral := range []bool{false, true} {
		t.Run(fmt.Sprintf("neutral=%v", neutral), func(t *testing.T) { testStaleOnlyOnRateLimitAndRefresh(t, neutral) })
	}
}
func testStaleOnlyOnRateLimitAndRefresh(t *testing.T, neutral bool) {
	for _, failure := range []string{"rate", "auth", "missing", "disabled"} {
		t.Run(failure, func(t *testing.T) {
			c := fake(t)
			if neutral {
				setManager(t, "[secrets]\nmanager = \"1password\"\nvault = \"example\"\nfield = \"token\"\n")
			}
			calls := fakeCalls(t)
			log := ReadLogPath()
			now := time.Now()
			c.now = func() time.Time { return now }
			r := refs()[:1]
			if neutral {
				r[0].URI = "secret://item"
			}
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
					uri, err := ExpandRef(r[0].URI)
					if err != nil {
						t.Fatal(err)
					}
					_, held := c.entries[uri]
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
	setPolicy(t, Policy{MeterHost: true, Daily: 100, Hourly: 100, CallerMinute: 5, Thresholds: []int{50, 80}, Notify: []string{exe, "-test.run=TestNotifyHelper", "--", out}})
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

func guardRef(neutral bool, name string) string {
	if neutral {
		return "secret://" + name
	}
	return "op://example/" + name + "/token"
}

func TestCallerBudgetWaitHonorsCancellationAndDeadline(t *testing.T) {
	c := fake(t)
	t.Setenv("HEV_SECRETS_CACHE", "off")
	const caller = "scope-test"
	now := time.Now()
	for range 5 {
		if err := reserve(now, caller+" direct", "op", 1); err != nil {
			t.Fatal(err)
		}
	}
	request := []Ref{{Name: "TOKEN", URI: "op://example/item/token"}}
	opts := Options{Caller: caller, CallerWait: time.Second, Notice: io.Discard}
	got, err := ResolveResult(context.Background(), request, opts)
	if !errors.Is(err, context.DeadlineExceeded) || got.Values != nil || got.Via != ViaDirect {
		t.Fatal("deadline must abort a queued direct read without returning credentials")
	}
	if c.LocalStatus().Budget.Throttles != 1 {
		t.Fatal("deadline case never entered the caller budget queue")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type outcome struct {
		got Result
		err error
	}
	result := make(chan outcome, 1)
	opts.CallerWait = time.Minute
	go func() {
		got, err := ResolveResult(ctx, request, opts)
		result <- outcome{got, err}
	}()
	for c.LocalStatus().Budget.Throttles < 2 {
		if err := waitForBudget(ctx, 10*time.Millisecond); err != nil {
			t.Fatal("cancellation case never entered the caller budget queue")
		}
	}
	cancel()
	select {
	case outcome := <-result:
		if !errors.Is(outcome.err, context.Canceled) || outcome.got.Values != nil {
			t.Fatal("cancellation must stop a queued read without returning credentials")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued read did not stop on cancellation")
	}
	if status := c.LocalStatus().Budget; status.UsedDay != 5 || status.Throttles != 2 {
		t.Fatal("waiting must neither reserve another read nor repeatedly probe the ledger")
	}
}

func TestCallerBudgetWaitThroughDaemon(t *testing.T) {
	c := fake(t)
	t.Setenv("HEV_SECRETS_CACHE", "")
	t.Setenv(EndpointEnv, isolatedEndpoint(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const caller = "scope-test"
	// Leave a short but real wait on the default rolling minute.
	spent := time.Now().Add(-57 * time.Second)
	for range 5 {
		if err := reserve(spent, caller, "op", 1); err != nil {
			t.Fatal(err)
		}
	}
	stop, err := Serve(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	got, err := ResolveResult(ctx, refs()[:1], Options{Caller: caller, CallerWait: 8 * time.Second, Notice: io.Discard})
	if err != nil || got.Via != ViaDaemon || got.Values["TOKEN"] == "" {
		t.Fatal("queued daemon resolution must succeed through the original endpoint")
	}
	if s := c.LocalStatus().Budget; s.UsedDay != 6 || s.Throttles < 1 {
		t.Fatal("daemon did not apply the default rolling budget")
	}
}

func TestCallerBudgetWaitAdmitsOlderReadersBeforeContinuations(t *testing.T) {
	fake(t)
	const caller = "shared-parent"
	now := time.Now()
	for range 5 {
		if err := reserve(now, caller, "op", 1); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(at time.Time, ticket string) error {
		ctx := context.WithValue(context.Background(), budgetTicketKey{}, ticket)
		return reserveWithContext(ctx, at, caller, "op", 1, nil)
	}
	for _, ticket := range []string{"first-cold-read", "second-cold-read"} {
		if err := wait(now, ticket); !IsBudgetError(err) {
			t.Fatal("cold read must queue when all five slots are charged")
		}
	}
	ready := now.Add(time.Minute)
	for _, ticket := range []string{"turn-continuation", "second-cold-read"} {
		if err := wait(ready, ticket); !IsBudgetError(err) {
			t.Fatal("a later reader overtook the oldest pending read")
		}
	}
	for _, ticket := range []string{"first-cold-read", "second-cold-read", "turn-continuation"} {
		if err := wait(ready, ticket); err != nil {
			t.Fatal("available budget must be admitted in registration order", err)
		}
	}
	if err := reserve(ready, caller, "op", 1); err != nil {
		t.Fatal("ordinary callers must still use available budget", err)
	}
	if err := reserve(ready, caller, "op", 1); err != nil {
		t.Fatal(err)
	}
	if err := wait(ready, "abandoned"); !IsBudgetError(err) {
		t.Fatal("queue must keep the five-read budget")
	}
	if err := wait(ready.Add(time.Minute+4*time.Second), "active"); err != nil {
		t.Fatal("an abandoned ticket must expire without blocking subsequent reads", err)
	}
}
