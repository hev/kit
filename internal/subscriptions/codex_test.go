package subscriptions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Synthetic clock for every test: 2030-03-06 12:00 UTC.
var testNow = time.Date(2030, 3, 6, 12, 0, 0, 0, time.UTC)

func testEnv(t *testing.T) Env {
	t.Helper()
	home := t.TempDir()
	return Env{
		Home:     home,
		StateDir: filepath.Join(home, ".hev"),
		Now:      func() time.Time { return testNow },
		Getenv:   func(string) string { return "" },
		Keychain: nil,
		// No test may reach the real endpoint.
		ClaudeUsageURL: "http://127.0.0.1:1/unused",
	}
}

func writeFile(t *testing.T, path, body string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if !mod.IsZero() {
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
}

// tokenCount renders one synthetic rollout token-count event.
func tokenCount(at time.Time, limits string) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10}},"rate_limits":%s}}`+"\n", at.Format(time.RFC3339Nano), limits)
}

func limits(id, plan string, primary, secondary string) string {
	return fmt.Sprintf(`{"limit_id":%q,"limit_name":null,"primary":%s,"secondary":%s,"credits":{"has_credits":true,"unlimited":false,"balance":"125.5000"},"plan_type":%q}`, id, primary, secondary, plan)
}

func window(used float64, minutes int, reset time.Time) string {
	return fmt.Sprintf(`{"used_percent":%v,"window_minutes":%d,"resets_at":%d}`, used, minutes, reset.Unix())
}

func codexTree(t *testing.T, env Env) {
	sessions := filepath.Join(env.Home, ".codex", "sessions")
	weekReset := testNow.Add(50 * time.Hour)
	sessionReset := testNow.Add(2 * time.Hour)
	// An old day the reader must not prefer.
	writeFile(t, filepath.Join(sessions, "2030", "02", "27", "rollout-a.jsonl"),
		tokenCount(testNow.Add(-7*24*time.Hour), limits("codex", "plus", window(90, 10080, weekReset), "null")), testNow.Add(-7*24*time.Hour))
	// Yesterday: a reading older than today's, in a file modified earlier.
	writeFile(t, filepath.Join(sessions, "2030", "03", "05", "rollout-b.jsonl"),
		`{"timestamp":"2030-03-05T10:00:00Z","type":"session_meta","payload":{}}`+"\n"+
			tokenCount(testNow.Add(-26*time.Hour), limits("codex", "pro", window(10, 300, sessionReset), window(20, 10080, weekReset))), testNow.Add(-26*time.Hour))
	// Today: the newest reading, followed by a token count with null limits
	// and a truncated final line, both of which are skipped.
	writeFile(t, filepath.Join(sessions, "2030", "03", "06", "rollout-c.jsonl"),
		tokenCount(testNow.Add(-90*time.Minute), limits("codex", "pro", window(12, 300, sessionReset), window(35, 10080, weekReset)))+
			tokenCount(testNow.Add(-30*time.Minute), limits("codex", "pro", window(14, 300, sessionReset), window(37.5, 10080, weekReset)))+
			`{"timestamp":"2030-03-06T11:45:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":null}}`+"\n"+
			`{"timestamp":"2030-03-06T11:50:00Z","type":"event_msg","payload":{"type":"token_cou`, testNow.Add(-10*time.Minute))
	// A model-scoped weekly limit reported under its own id.
	writeFile(t, filepath.Join(sessions, "2030", "03", "06", "rollout-d.jsonl"),
		tokenCount(testNow.Add(-40*time.Minute), `{"limit_id":"codex_fixture_model","limit_name":"Fixture Model","primary":`+window(5, 10080, weekReset)+`,"secondary":null,"plan_type":"pro"}`), testNow.Add(-40*time.Minute))
	writeFile(t, filepath.Join(env.Home, ".codex", "auth.json"), `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"access_token":"synthetic-not-a-token"}}`, time.Time{})
}

func TestCodexChoosesNewestReading(t *testing.T) {
	env := testEnv(t)
	codexTree(t, env)
	d, err := ReadOne(context.Background(), env, "codex")
	if err != nil {
		t.Fatal(err)
	}
	s := d.Subscription
	if s.State != SignedIn || s.Plan == nil || *s.Plan != "pro" {
		t.Fatalf("state/plan = %s/%v", s.State, s.Plan)
	}
	if s.ObservedAt == nil || !s.ObservedAt.Equal(testNow.Add(-30*time.Minute)) || *s.AgeSeconds != 1800 {
		t.Fatalf("observed = %v age %v", s.ObservedAt, s.AgeSeconds)
	}
	if s.Weekly == nil || *s.Weekly.Percent != 37.5 || !s.Weekly.ResetsAt.Equal(testNow.Add(50*time.Hour)) || s.Weekly.RolledOver {
		t.Fatalf("weekly = %+v", s.Weekly)
	}
	got := map[string]Meter{}
	for _, m := range s.Meters {
		got[m.Label] = m
	}
	if m := got["session (5h)"]; m.Kind != KindSession || m.Percent == nil || *m.Percent != 14 {
		t.Fatalf("session = %+v (all %+v)", m, s.Meters)
	}
	if m := got["credits"]; m.Kind != KindCredits || m.Percent != nil || m.Detail != "balance 125.5" || m.PercentReason == "" {
		t.Fatalf("credits = %+v", m)
	}
	if m := got["weekly (Fixture Model)"]; m.Kind != KindWeeklyScoped || m.Scope != "Fixture Model" || *m.Percent != 5 {
		t.Fatalf("scoped = %+v", m)
	}
	if s.Stale {
		t.Fatalf("fresh reading marked stale: %s", s.StaleReason)
	}
}

func TestCodexReadingPastResetRollsOver(t *testing.T) {
	env := testEnv(t)
	sessions := filepath.Join(env.Home, ".codex", "sessions")
	reset := testNow.Add(-time.Hour)
	writeFile(t, filepath.Join(sessions, "2030", "03", "06", "rollout.jsonl"),
		tokenCount(testNow.Add(-3*time.Hour), limits("codex", "plus", window(88, 10080, reset), "null")), testNow.Add(-3*time.Hour))
	writeFile(t, filepath.Join(env.Home, ".codex", "auth.json"), `{"auth_mode":"chatgpt"}`, time.Time{})
	d, err := ReadOne(context.Background(), env, "codex")
	if err != nil {
		t.Fatal(err)
	}
	w := d.Subscription.Weekly
	if w == nil || !w.RolledOver || w.Percent == nil || *w.Percent != 0 || w.ResetsAt != nil || !strings.Contains(w.ResetReason, reset.Format(time.RFC3339)) {
		t.Fatalf("weekly = %+v", w)
	}
	if *d.Subscription.Plan != "plus" {
		t.Fatalf("plan = %v", *d.Subscription.Plan)
	}
	var out strings.Builder
	if err := WriteDetail(&out, d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "rolled over") {
		t.Fatalf("detail does not say rolled over:\n%s", out.String())
	}
}

func TestCodexLoginStates(t *testing.T) {
	ctx := context.Background()
	t.Run("not signed in", func(t *testing.T) {
		env := testEnv(t)
		s, _ := ReadOne(ctx, env, "codex")
		if s.Subscription.State != NotSignedIn || s.Subscription.Weekly != nil || s.Subscription.WeeklyReason == "" {
			t.Fatalf("%+v", s.Subscription)
		}
	})
	t.Run("api key is not a plan", func(t *testing.T) {
		env := testEnv(t)
		writeFile(t, filepath.Join(env.Home, ".codex", "auth.json"), `{"auth_mode":"apikey"}`, time.Time{})
		s, _ := ReadOne(ctx, env, "codex")
		if s.Subscription.State != NotSignedIn || !strings.Contains(s.Subscription.StateReason, "API key") {
			t.Fatalf("%+v", s.Subscription)
		}
	})
	t.Run("signed in with no reading", func(t *testing.T) {
		env := testEnv(t)
		writeFile(t, filepath.Join(env.Home, ".codex", "auth.json"), `{"auth_mode":"chatgpt"}`, time.Time{})
		s, _ := ReadOne(ctx, env, "codex")
		if s.Subscription.State != SignedIn || s.Subscription.ObservedAt != nil || s.Subscription.ObservedReason == "" {
			t.Fatalf("%+v", s.Subscription)
		}
	})
	t.Run("CODEX_HOME", func(t *testing.T) {
		env := testEnv(t)
		alt := t.TempDir()
		env.Getenv = func(k string) string {
			if k == "CODEX_HOME" {
				return alt
			}
			return ""
		}
		writeFile(t, filepath.Join(alt, "auth.json"), `{"auth_mode":"chatgpt"}`, time.Time{})
		s, _ := ReadOne(ctx, env, "codex")
		if s.Subscription.State != SignedIn {
			t.Fatalf("%+v", s.Subscription)
		}
	})
}

func TestCodexReadsOnlyRecentRollouts(t *testing.T) {
	env := testEnv(t)
	sessions := filepath.Join(env.Home, ".codex", "sessions")
	reset := testNow.Add(24 * time.Hour)
	// The newest reading sits in an old day directory beyond the window; the
	// reader must not walk that far back.
	writeFile(t, filepath.Join(sessions, "2029", "12", "01", "old.jsonl"),
		tokenCount(testNow.Add(-time.Minute), limits("codex", "old", window(99, 10080, reset), "null")), testNow.Add(-time.Minute))
	for day := 1; day <= codexDays; day++ {
		writeFile(t, filepath.Join(sessions, "2030", "03", fmt.Sprintf("%02d", day), "r.jsonl"),
			tokenCount(testNow.Add(-time.Duration(8-day)*time.Hour), limits("codex", "pro", window(float64(day), 10080, reset), "null")), testNow.Add(-time.Duration(8-day)*time.Hour))
	}
	d, _ := ReadOne(context.Background(), env, "codex")
	if *d.Subscription.Plan != "pro" || *d.Subscription.Weekly.Percent != float64(codexDays) {
		t.Fatalf("plan %v weekly %+v", *d.Subscription.Plan, d.Subscription.Weekly)
	}
}
