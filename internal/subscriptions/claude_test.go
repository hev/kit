package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Synthetic credential material. Nothing here is or resembles a real token.
const fixtureToken = "fixture-oauth-token-0000"

func credentialJSON(expires time.Time) string {
	return `{"claudeAiOauth":{"accessToken":"` + fixtureToken + `","refreshToken":"fixture-refresh-0000","expiresAt":` +
		jsonInt(expires.UnixMilli()) + `,"scopes":["user:inference"],"subscriptionType":"max","rateLimitTier":"default_claude_max_5x"}}`
}

func jsonInt(v int64) string { b, _ := json.Marshal(v); return string(b) }

// usageBody is the recorded shape of the usage response, with synthetic values.
var usageBody = `{
  "limits": [
    {"kind": "session", "percent": 18, "resets_at": "2030-03-06T15:00:00Z"},
    {"kind": "weekly_all", "percent": 41.5, "resets_at": "2030-03-09T18:00:00Z"},
    {"kind": "weekly_scoped", "percent": null, "resets_at": "2030-03-09T18:00:00Z", "scope": {"model": {"display_name": "Fixture Large"}}}
  ],
  "extra_usage": {"is_enabled": true, "percent": 0, "resets_at": "2030-04-01T00:00:00Z"}
}`

type usageServer struct {
	*httptest.Server
	hits   atomic.Int32
	status atomic.Int32
	body   string
	auth   atomic.Value
}

func newUsageServer(t *testing.T, body string) *usageServer {
	u := &usageServer{body: body}
	u.status.Store(http.StatusOK)
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)
		u.auth.Store(r.Header.Get("Authorization") + "|" + r.Header.Get("anthropic-beta"))
		code := int(u.status.Load())
		w.WriteHeader(code)
		if code == http.StatusOK {
			_, _ = w.Write([]byte(u.body))
		} else {
			_, _ = w.Write([]byte(`{"error":{"type":"fixture"}}`))
		}
	}))
	t.Cleanup(u.Close)
	return u
}

func claudeEnv(t *testing.T, u *usageServer) Env {
	env := testEnv(t)
	writeFile(t, filepath.Join(env.Home, ".claude", ".credentials.json"), credentialJSON(testNow.Add(time.Hour)), time.Time{})
	if u != nil {
		env.ClaudeUsageURL = u.URL + "/api/oauth/usage"
		env.HTTP = u.Client()
	}
	return env
}

func readClaudeDetail(t *testing.T, env Env) Status {
	t.Helper()
	d, err := ReadOne(context.Background(), env, "claude")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), fixtureToken) || strings.Contains(string(b), "fixture-refresh") {
		t.Fatalf("status leaks the credential: %s", b)
	}
	return d.Subscription
}

func TestClaudeMetersFromRecordedShape(t *testing.T) {
	u := newUsageServer(t, usageBody)
	env := claudeEnv(t, u)
	s := readClaudeDetail(t, env)
	if u.hits.Load() != 1 {
		t.Fatalf("hits = %d", u.hits.Load())
	}
	if got := u.auth.Load().(string); got != "Bearer "+fixtureToken+"|oauth-2025-04-20" {
		t.Fatal("request did not carry the bearer token and beta header")
	}
	if s.State != SignedIn || s.Plan == nil || *s.Plan != "max 5x" || s.Stale {
		t.Fatalf("state %s plan %v stale %v", s.State, s.Plan, s.Stale)
	}
	if s.Weekly == nil || *s.Weekly.Percent != 41.5 || s.Weekly.ResetsAt.Format(time.RFC3339) != "2030-03-09T18:00:00Z" {
		t.Fatalf("weekly = %+v", s.Weekly)
	}
	kinds := []string{}
	for _, m := range s.Meters {
		kinds = append(kinds, m.Kind)
	}
	if strings.Join(kinds, ",") != "session,weekly,weekly_scoped,extra_usage" {
		t.Fatalf("kinds = %v", kinds)
	}
	scoped := s.Meters[2]
	if scoped.Scope != "Fixture Large" || scoped.Label != "weekly (Fixture Large)" || scoped.Percent != nil || scoped.PercentReason == "" {
		t.Fatalf("scoped = %+v", scoped)
	}
	if s.ObservedAt == nil || *s.AgeSeconds != 0 {
		t.Fatalf("observed %v", s.ObservedAt)
	}
}

func TestClaudeKeyedUsageShape(t *testing.T) {
	u := newUsageServer(t, `{"five_hour":{"utilization":7,"resets_at":"2030-03-06T14:00:00+00:00"},"seven_day":{"utilization":63,"resets_at":"2030-03-10T09:00:00+00:00"},"seven_day_opus":{"utilization":12,"resets_at":null},"seven_day_oauth_apps":null,"extra_usage":{"is_enabled":false}}`)
	s := readClaudeDetail(t, claudeEnv(t, u))
	if s.Weekly == nil || *s.Weekly.Percent != 63 {
		t.Fatalf("weekly = %+v", s.Weekly)
	}
	labels := []string{}
	for _, m := range s.Meters {
		labels = append(labels, m.Label)
	}
	if strings.Join(labels, ",") != "session (5h),weekly,weekly (Opus),extra usage" {
		t.Fatalf("labels = %v", labels)
	}
	if s.Meters[2].ResetsAt != nil || s.Meters[2].ResetReason == "" || s.Meters[3].Detail != "off" {
		t.Fatalf("meters = %+v", s.Meters)
	}
}

func TestClaudeUnknownPercentIsNullWithReason(t *testing.T) {
	u := newUsageServer(t, `{"limits":[{"kind":"weekly_all","percent":null,"resets_at":"2030-03-09T18:00:00Z"}]}`)
	d, err := ReadOne(context.Background(), claudeEnv(t, u), "claude")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	var raw struct {
		Subscription struct {
			Weekly map[string]any `json:"weekly"`
		} `json:"subscription"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	p, present := raw.Subscription.Weekly["percent"]
	if !present || p != nil {
		t.Fatalf("percent = %#v present %v in %s", p, present, b)
	}
	if r, _ := raw.Subscription.Weekly["percent_reason"].(string); r == "" {
		t.Fatalf("no reason in %s", b)
	}
	var out strings.Builder
	_ = WriteList(&out, List{GeneratedAt: testNow, Subscriptions: []Status{d.Subscription}})
	if !strings.Contains(out.String(), "?") || strings.Contains(out.String(), " 0%") {
		t.Fatalf("list shows an unknown as a number:\n%s", out.String())
	}
}

func TestClaude401IsExpiredLogin(t *testing.T) {
	u := newUsageServer(t, usageBody)
	u.status.Store(http.StatusUnauthorized)
	env := claudeEnv(t, u)
	s := readClaudeDetail(t, env)
	if s.State != LoginExpired || u.hits.Load() != 1 {
		t.Fatalf("state %s hits %d", s.State, u.hits.Load())
	}
	// Inside the interval the expired state is remembered without asking.
	s = readClaudeDetail(t, env)
	if s.State != LoginExpired || u.hits.Load() != 1 {
		t.Fatalf("second read: state %s hits %d", s.State, u.hits.Load())
	}
}

func TestClaude429ServesCacheMarkedStale(t *testing.T) {
	u := newUsageServer(t, usageBody)
	env := claudeEnv(t, u)
	readClaudeDetail(t, env)
	// Move past the interval, then rate limit.
	later := testNow.Add(DefaultMinInterval + time.Minute)
	env.Now = func() time.Time { return later }
	u.status.Store(http.StatusTooManyRequests)
	u.hits.Store(0)
	s := readClaudeDetail(t, env)
	if u.hits.Load() != 1 {
		t.Fatalf("429 read made %d requests, want exactly 1", u.hits.Load())
	}
	if !s.Stale || !strings.Contains(s.StaleReason, "429") || s.State != SignedIn {
		t.Fatalf("stale %v reason %q state %s", s.Stale, s.StaleReason, s.State)
	}
	if s.Weekly == nil || *s.Weekly.Percent != 41.5 || *s.AgeSeconds != int64((DefaultMinInterval+time.Minute).Seconds()) {
		t.Fatalf("cached weekly %+v age %v", s.Weekly, s.AgeSeconds)
	}
	// The failure is remembered: another read inside the interval asks nothing.
	s = readClaudeDetail(t, env)
	if u.hits.Load() != 1 || !s.Stale || !strings.Contains(s.StaleReason, "429") {
		t.Fatalf("hits %d stale %v %q", u.hits.Load(), s.Stale, s.StaleReason)
	}
}

func TestClaude429WithoutCacheIsUnknown(t *testing.T) {
	u := newUsageServer(t, usageBody)
	u.status.Store(http.StatusTooManyRequests)
	s := readClaudeDetail(t, claudeEnv(t, u))
	if s.Weekly != nil || s.ObservedAt != nil || !s.Stale || u.hits.Load() != 1 {
		t.Fatalf("%+v hits %d", s, u.hits.Load())
	}
}

func TestClaudeTwoReadsInsideIntervalMakeOneRequest(t *testing.T) {
	u := newUsageServer(t, usageBody)
	env := claudeEnv(t, u)
	first := readClaudeDetail(t, env)
	later := testNow.Add(DefaultMinInterval - time.Second)
	env.Now = func() time.Time { return later }
	second := readClaudeDetail(t, env)
	if u.hits.Load() != 1 {
		t.Fatalf("two reads made %d requests", u.hits.Load())
	}
	if second.Stale || second.Source != "Claude OAuth usage (cached)" || *second.AgeSeconds != int64((DefaultMinInterval-time.Second).Seconds()) || *second.Weekly.Percent != *first.Weekly.Percent {
		t.Fatalf("second = %+v", second)
	}
	b, err := os.ReadFile(ClaudeCachePath(env))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{fixtureToken, "fixture-refresh", "Bearer", "accessToken"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("cache contains %q:\n%s", secret, b)
		}
	}
	if info, _ := os.Stat(ClaudeCachePath(env)); info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("cache mode %v", info.Mode())
	}
	// --refresh forces one more request.
	env.Refresh = true
	readClaudeDetail(t, env)
	if u.hits.Load() != 2 {
		t.Fatalf("refresh made %d total requests", u.hits.Load())
	}
}

func TestClaudeLoginStates(t *testing.T) {
	t.Run("not signed in", func(t *testing.T) {
		u := newUsageServer(t, usageBody)
		env := testEnv(t)
		env.ClaudeUsageURL, env.HTTP = u.URL, u.Client()
		s := readClaudeDetail(t, env)
		if s.State != NotSignedIn || u.hits.Load() != 0 || s.Weekly != nil || s.WeeklyReason != "not signed in" {
			t.Fatalf("%+v hits %d", s, u.hits.Load())
		}
	})
	t.Run("expired token makes no request", func(t *testing.T) {
		u := newUsageServer(t, usageBody)
		env := claudeEnv(t, u)
		writeFile(t, filepath.Join(env.Home, ".claude", ".credentials.json"), credentialJSON(testNow.Add(-time.Minute)), time.Time{})
		s := readClaudeDetail(t, env)
		if s.State != LoginExpired || u.hits.Load() != 0 {
			t.Fatalf("%+v hits %d", s, u.hits.Load())
		}
	})
	t.Run("environment token", func(t *testing.T) {
		u := newUsageServer(t, usageBody)
		env := testEnv(t)
		env.ClaudeUsageURL, env.HTTP = u.URL, u.Client()
		env.Getenv = func(k string) string {
			if k == "CLAUDE_CODE_OAUTH_TOKEN" {
				return fixtureToken
			}
			return ""
		}
		s := readClaudeDetail(t, env)
		if s.State != SignedIn || s.Plan != nil || !strings.Contains(s.PlanReason, "CLAUDE_CODE_OAUTH_TOKEN") {
			t.Fatalf("%+v", s)
		}
	})
	t.Run("keychain before file", func(t *testing.T) {
		u := newUsageServer(t, usageBody)
		env := testEnv(t)
		env.ClaudeUsageURL, env.HTTP = u.URL, u.Client()
		env.Keychain = func(context.Context) ([]byte, error) {
			return []byte(strings.Replace(credentialJSON(testNow.Add(time.Hour)), `"max"`, `"pro"`, 1)), nil
		}
		s := readClaudeDetail(t, env)
		if s.State != SignedIn || *s.Plan != "pro 5x" {
			t.Fatalf("%+v", s)
		}
	})
	t.Run("unreadable credential", func(t *testing.T) {
		env := testEnv(t)
		writeFile(t, filepath.Join(env.Home, ".claude", ".credentials.json"), `{"claudeAiOauth":`+fixtureToken, time.Time{})
		s := readClaudeDetail(t, env)
		if s.State != Unknown || strings.Contains(s.StateReason, fixtureToken) {
			t.Fatalf("%+v", s)
		}
	})
}

func TestClaudeOfflineListStillUseful(t *testing.T) {
	u := newUsageServer(t, usageBody)
	env := claudeEnv(t, u)
	readClaudeDetail(t, env)
	u.Close() // the network is gone
	env.Now = func() time.Time { return testNow.Add(30 * time.Minute) }
	l := ReadAll(context.Background(), env)
	var claude Status
	for _, s := range l.Subscriptions {
		if s.Name == "claude" {
			claude = s
		}
	}
	if claude.Weekly == nil || !claude.Stale || !strings.Contains(claude.StaleReason, "usage request failed") || strings.Contains(claude.StaleReason, fixtureToken) {
		t.Fatalf("%+v", claude)
	}
	var out strings.Builder
	if err := WriteList(&out, l); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "(stale)") {
		t.Fatalf("list:\n%s", out.String())
	}
}

func TestCachedReadingPastResetRollsOver(t *testing.T) {
	u := newUsageServer(t, usageBody)
	env := claudeEnv(t, u)
	readClaudeDetail(t, env)
	u.status.Store(http.StatusTooManyRequests)
	// Four days on, the weekly reset (2030-03-09T18:00Z) has passed.
	env.Now = func() time.Time { return testNow.Add(96 * time.Hour) }
	writeFile(t, filepath.Join(env.Home, ".claude", ".credentials.json"), credentialJSON(testNow.Add(100*time.Hour)), time.Time{})
	s := readClaudeDetail(t, env)
	if s.Weekly == nil || !s.Weekly.RolledOver || *s.Weekly.Percent != 0 {
		t.Fatalf("weekly = %+v", s.Weekly)
	}
}

func TestUnknownName(t *testing.T) {
	_, err := ReadOne(context.Background(), testEnv(t), "gemini")
	var u UnknownNameError
	if !errors.As(err, &u) || !strings.Contains(err.Error(), "claude, codex") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfiguredPlansNamed(t *testing.T) {
	env := testEnv(t)
	path := filepath.Join(env.Home, "subscriptions.toml")
	writeFile(t, path, "[[plans]]\nname='Fixture codex plan'\nmonthly_usd=1\nreset_day=1\nharness='codex'\n", time.Time{})
	plans, err := LoadPlans(path)
	if err != nil {
		t.Fatal(err)
	}
	env.Plans = plans
	l := ReadAll(context.Background(), env)
	for _, s := range l.Subscriptions {
		want := map[string]string{"claude": "", "codex": "Fixture codex plan"}[s.Name]
		if strings.Join(s.ConfiguredPlans, ",") != want {
			t.Fatalf("%s configured = %v", s.Name, s.ConfiguredPlans)
		}
	}
}
