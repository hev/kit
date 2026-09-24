package cost

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoricalRatesAndWholeWeekAllocation(t *testing.T) {
	ts := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC).UnixMilli()
	c := Config{Rates: []Rate{{Model: "test", Effective: "2026-01-01", Input: 2, Cached: 1, Output: 4}, {Model: "test", Effective: "2026-10-01", Input: 20}}, Plans: []Plan{{Name: "test", Harness: "codex", Monthly: dollarPointer(100), Reset: 1}}}
	rows := []Row{{ID: "a", Harness: "codex", Start: ts, Usage: map[string]Usage{"test": {Input: 100, Cached: 100, Output: 100, Reasoning: 50}}}, {ID: "b", Harness: "codex", Start: ts, Role: "other", Usage: map[string]Usage{"test": {Input: 900}}}}
	if err := c.Price(rows); err != nil {
		t.Fatal(err)
	}
	if math.Abs(*rows[0].API-.0007) > 1e-10 || rows[0].Tokens != 300 {
		t.Fatalf("bad API accounting: %+v", rows[0])
	}
	weekly := 100.0 * 12 / 365.2425 * 7
	if math.Abs(*rows[0].Sub-weekly/4) > 1e-10 || math.Abs(*rows[0].Sub+*rows[1].Sub-weekly) > 1e-10 {
		t.Fatal("allocation failed conservation")
	}
}
func TestUnknownPriceAndOverlappingPlans(t *testing.T) {
	rows := []Row{{ID: "a", Harness: "codex", Usage: map[string]Usage{"unknown": {Input: 1}}}}
	c := Config{}
	if err := c.Price(rows); err != nil || rows[0].API != nil {
		t.Fatal("unknown price must remain unknown")
	}
	c.Plans = []Plan{{Name: "a", Harness: "codex"}, {Name: "b", Harness: "codex"}}
	if c.Price(rows) == nil {
		t.Fatal("ambiguous plan accepted")
	}
}
func TestOutcomeScope(t *testing.T) {
	p := filepath.Join(t.TempDir(), "outcomes.json")
	if err := os.WriteFile(p, []byte(`{"team":"example","refreshed_at":1,"issues":[{"team":"example","issue":"EX-1","prs":[{"repo":"outside/repo"}]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOutcomes(p, "example", []string{"example/repo"}, time.Now()); err == nil {
		t.Fatal("scope escape accepted")
	}
}

func TestMeasuredWeeklyUsageAndStaleness(t *testing.T) {
	now := time.Now()
	pct := 77.0
	c := Config{Plans: []Plan{{Name: "codex", Harness: "codex"}}}
	rows := []Row{{Subscription: "codex", End: now.UnixMilli(), Limits: []byte(`{"primary":{"used_percent":15,"window_minutes":300,"resets_at":1},"secondary":{"used_percent":77,"window_minutes":10080,"resets_at":9999999999}}`)}}
	limits := c.Limits(rows, now)
	if len(limits) != 1 || limits[0].Percent == nil || *limits[0].Percent != pct || limits[0].Stale {
		t.Fatal(limits)
	}
	rows[0].End = now.Add(-2 * time.Hour).UnixMilli()
	if !c.Limits(rows, now)[0].Stale {
		t.Fatal("old observation not stale")
	}
}

func TestClaudeUsageCacheIsMeasuredAndScopedToPlan(t *testing.T) {
	now := time.Now()
	p := filepath.Join(t.TempDir(), "claude-usage.json")
	raw := fmt.Sprintf(`{"plan":"claude","observed_at":%d,"limits":[{"kind":"weekly_all","percent":77,"resets_at":%q}]}`, now.UnixMilli(), now.Add(24*time.Hour).Format(time.RFC3339))
	if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	limits := []Limit{{Name: "claude", Source: "unavailable"}}
	if err := ApplyClaudeUsage(p, limits, now); err != nil {
		t.Fatal(err)
	}
	if limits[0].Percent == nil || *limits[0].Percent != 77 || limits[0].Stale || limits[0].Source != "Claude OAuth usage" {
		t.Fatal(limits)
	}
	if ApplyClaudeUsage(p, []Limit{{Name: "another-plan"}}, now) == nil {
		t.Fatal("unconfigured plan accepted")
	}
}

func dollarPointer(v float64) *float64 { return &v }

func TestRequestDatedLongContextAndUnknownSubscription(t *testing.T) {
	ts := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC).UnixMilli()
	c := Config{Rates: []Rate{{Model: "fixture", Effective: "2026-09-08", Input: 2, Output: 4, Creation1h: dollarPointer(5), LongContext: 272000, LongInput: 2, LongOutput: 1.5}}, Plans: []Plan{{Name: "unknown-bill", Harness: "codex"}}}
	u := Usage{Input: 300000, Creation1h: 10, Output: 100}
	rows := []Row{{ID: "r", Harness: "codex", Usage: map[string]Usage{"fixture": u}, Requests: []Request{{Model: "fixture", TS: ts, Usage: u}}}}
	if err := c.Price(rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].API == nil || math.Abs(*rows[0].API-1.2007) > 1e-9 {
		t.Fatalf("wrong long-context/cache price: %+v", rows[0])
	}
	if rows[0].Sub != nil {
		t.Fatal("unknown bill became free subscription")
	}
	c.Rates = append(c.Rates, Rate{Model: "fixture", Effective: "2026-09-09", Input: 4})
	rows[0].Requests = []Request{{Model: "fixture", TS: ts, Usage: Usage{Input: 1}}, {Model: "fixture", TS: ts + 86400000, Usage: Usage{Input: 1}}}
	if err := c.Price(rows); err != nil {
		t.Fatal(err)
	}
	if math.Abs(*rows[0].API-.000006) > 1e-12 {
		t.Fatal("request date did not select historical rate")
	}
}

func TestCurrentWeekUsesPlanResetInsteadOfViewWindow(t *testing.T) {
	p := Plan{Name: "example", Reset: 2, ResetHour: 2, ResetMinute: 59, ResetSecond: 53}
	c := Config{Plans: []Plan{p}}
	reset := time.Date(2026, 9, 8, 2, 59, 53, 0, time.UTC)
	if c.InCurrentWeek(Row{Subscription: p.Name, Start: reset.Add(-time.Second).UnixMilli()}, reset.Add(time.Hour)) {
		t.Fatal("prior plan week included")
	}
	if !c.InCurrentWeek(Row{Subscription: p.Name, Start: reset.UnixMilli()}, reset.Add(time.Hour)) {
		t.Fatal("current plan week excluded")
	}
	if c.InCurrentWeek(Row{Start: reset.UnixMilli()}, reset.Add(time.Hour)) {
		t.Fatal("unconfigured plan inferred")
	}
}

func TestShippedDatedRates(t *testing.T) {
	c, err := LoadConfig("../../prices.toml", "../../subscriptions.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		model, date string
		usage       Usage
		want        *float64
	}{
		{"claude-sonnet-5", "2026-06-29", Usage{Input: 1000000}, nil},
		{"claude-sonnet-5", "2026-06-30", Usage{Input: 1000000, Cached: 1000000, Creation: 1000000, Output: 1000000}, dollarPointer(14.7)},
		{"claude-sonnet-5", "2026-08-22", Usage{Creation1h: 1000000}, nil},
		{"claude-sonnet-5", "2026-08-23", Usage{Creation1h: 1000000}, dollarPointer(4)},
		{"claude-sonnet-5", "2026-09-01", Usage{Input: 1000000, Output: 1000000}, dollarPointer(12)},
		{"claude-fable-5-1", "2026-08-31", Usage{Cached: 1000000}, nil},
		{"claude-fable-5-1", "2026-09-01", Usage{Cached: 1000000, Creation1h: 1000000}, dollarPointer(20.25)},
		{"claude-haiku-4-5-20251001", "2025-10-01", Usage{Input: 1000000}, nil},
		{"claude-haiku-4-5-20251001", "2025-10-15", Usage{Input: 1000000}, dollarPointer(1)},
	} {
		t.Run(tc.model+tc.date, func(t *testing.T) {
			ts, _ := time.Parse("2006-01-02", tc.date)
			rows := []Row{{ID: "fixture", Start: ts.UnixMilli(), Usage: map[string]Usage{tc.model: tc.usage}, Requests: []Request{{Model: tc.model, TS: ts.UnixMilli(), Usage: tc.usage}}}}
			if err := c.Price(rows); err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				if rows[0].API != nil {
					t.Fatal("unsupported date/category was priced")
				}
				return
			}
			if rows[0].API == nil || math.Abs(*rows[0].API-*tc.want) > 1e-9 {
				t.Fatalf("got %v want %v", rows[0].API, tc.want)
			}
		})
	}
}

func TestGlobalDeploymentScope(t *testing.T) {
	p := filepath.Join(t.TempDir(), "outcomes.json")
	if err := os.WriteFile(p, []byte(`{"team":"example","refreshed_at":1,"issues":[],"deploys":[{"id":"fixture","repo":"outside/repo","landed_at":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOutcomes(p, "example", []string{"example/repo"}, time.Now()); err == nil {
		t.Fatal("global deployment escaped scope")
	}
}
