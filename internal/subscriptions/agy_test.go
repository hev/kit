package subscriptions

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/internal/platformops"
)

const (
	fixtureAgyAccess  = "ya29.fixture-agy-access-token-9999"
	fixtureAgyRefresh = "1//0gFixtureAgyRefreshToken8888"
	fixtureAgyAPIKey  = "AIzaFixtureGeminiApiKey0123456789"
	fixtureAgyEmail   = "operator-secret@example.com"
	fixtureAgySecret  = "op://layer-factory/gemini/GEMINI_API_KEY"
)

func agyTokenJSON(expiry time.Time, withRefresh bool, authMethod string) string {
	refresh := ""
	if withRefresh {
		refresh = fixtureAgyRefresh
	}
	return fmt.Sprintf(`{"token":{"access_token":%q,"refresh_token":%q,"token_type":"Bearer","expiry":%q},"auth_method":%q,"id_token_email":%q}`,
		fixtureAgyAccess, refresh, expiry.Format(time.RFC3339Nano), authMethod, fixtureAgyEmail)
}

func writeAgyStepTranscript(t *testing.T, root, cid, filename string, mod time.Time, lines ...string) {
	t.Helper()
	p := filepath.Join(root, "brain", cid, ".system_generated", "logs", filename)
	writeFile(t, p, strings.Join(lines, "\n")+"\n", mod)
}

func TestAgyAuthAndPlanStates(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(t *testing.T, env *Env, dataDir string)
		wantState   string
		wantPlan    string // "" means nil
		wantWeekly  bool
		wantSession bool
		wantReason  string
	}{
		{
			name: "oauth ultra via agy models",
			setup: func(t *testing.T, env *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
				env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
					return AgyProbeResult{
						Installed: true,
						Models:    []string{"gemini-4-neon-high", "gemini-4-carbon-high", "gemini-pro-orchestrator", "gemini-3.8-flash-high"},
					}, nil
				}
			},
			wantState:   SignedIn,
			wantPlan:    "ultra",
			wantWeekly:  true,
			wantSession: true,
		},
		{
			name: "oauth pro via agy models without carbon",
			setup: func(t *testing.T, env *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
				env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
					return AgyProbeResult{
						Installed: true,
						Models:    []string{"gemini-4-neon-medium", "gemini-4-argon-high", "gemini-4-hydrogen-low", "gemini-3.8-flash-high"},
					}, nil
				}
			},
			wantState:   SignedIn,
			wantPlan:    "pro",
			wantWeekly:  true,
			wantSession: true,
		},
		{
			name: "oauth standard via flash-only model list",
			setup: func(t *testing.T, env *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
				env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
					return AgyProbeResult{
						Installed: true,
						Models:    []string{"gemini-3.8-flash-high", "gemini-3.8-flash-low", "gpt-oss-120b-medium"},
					}, nil
				}
			},
			wantState:   SignedIn,
			wantPlan:    "standard",
			wantWeekly:  true,
			wantSession: false,
		},
		{
			name: "oauth enterprise via auth_method",
			setup: func(t *testing.T, env *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "enterprise"), testNow)
			},
			wantState:   SignedIn,
			wantPlan:    "enterprise",
			wantWeekly:  true,
			wantSession: true,
		},
		{
			name: "oauth signed in via OS keyring without token file",
			setup: func(t *testing.T, env *Env, _ string) {
				env.AgyKeychain = func(context.Context) (bool, error) { return true, nil }
				env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
					return AgyProbeResult{Installed: true, Models: []string{"gemini-4-carbon-medium"}}, nil
				}
			},
			wantState:   SignedIn,
			wantPlan:    "ultra",
			wantWeekly:  true,
			wantSession: true,
		},
		{
			name: "oauth past access expiry stays signed_in when refresh_token exists",
			setup: func(t *testing.T, env *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(-2*time.Hour), true, "consumer"), testNow)
				env.Plans = []ConfiguredPlan{{Name: "agy-ultra", Harness: "agy", Tier: "ultra"}}
			},
			wantState:   SignedIn,
			wantPlan:    "ultra",
			wantWeekly:  true,
			wantSession: true,
		},
		{
			name: "oauth past access expiry without refresh_token is login_expired",
			setup: func(t *testing.T, _ *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(-10*time.Minute), false, "consumer"), testNow)
			},
			wantState:  LoginExpired,
			wantReason: "expired",
		},
		{
			name: "stored token rejected by agy models is login_expired",
			setup: func(t *testing.T, env *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
				env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
					return AgyProbeResult{Installed: true, SignedOut: true}, nil
				}
			},
			wantState:  LoginExpired,
			wantReason: "rejected",
		},
		{
			name: "api-key mode via settings.json modelProvider=gemini overrides oauth file",
			setup: func(t *testing.T, _ *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
				writeFile(t, filepath.Join(dataDir, "settings.json"), `{"modelProvider":"gemini"}`, testNow)
			},
			wantState:  NotSignedIn,
			wantPlan:   "api-key",
			wantReason: "API-key billing",
		},
		{
			name: "api-key mode via dispatch.toml model_provider=gemini",
			setup: func(t *testing.T, env *Env, _ string) {
				cfg := filepath.Join(env.Home, ".config", "hev", "dispatch.toml")
				writeFile(t, cfg, "[harnesses.agy]\nmodel_provider = \"gemini\"\n[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+fixtureAgySecret+"\"\n", testNow)
			},
			wantState:  NotSignedIn,
			wantPlan:   "api-key",
			wantReason: "dispatch.toml",
		},
		{
			name: "api-key mode via dispatch.toml env_refs GEMINI_API_KEY alone",
			setup: func(t *testing.T, env *Env, _ string) {
				cfg := filepath.Join(env.Home, ".config", "hev", "dispatch.toml")
				writeFile(t, cfg, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+fixtureAgySecret+"\"\n", testNow)
			},
			wantState:  NotSignedIn,
			wantPlan:   "api-key",
			wantReason: "GEMINI_API_KEY",
		},
		{
			name: "api-key mode via GEMINI_API_KEY env var",
			setup: func(t *testing.T, env *Env, _ string) {
				env.Getenv = func(k string) string {
					if k == "GEMINI_API_KEY" {
						return fixtureAgyAPIKey
					}
					return ""
				}
			},
			wantState:  NotSignedIn,
			wantPlan:   "api-key",
			wantReason: "GEMINI_API_KEY",
		},
		{
			name: "signed out when agy installed with no credentials",
			setup: func(t *testing.T, env *Env, dataDir string) {
				if err := os.MkdirAll(dataDir, 0o700); err != nil {
					t.Fatal(err)
				}
				env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
					return AgyProbeResult{Installed: true, SignedOut: true}, nil
				}
			},
			wantState:  NotSignedIn,
			wantReason: "not signed in",
		},
		{
			name: "missing agy binary and no data dir",
			setup: func(t *testing.T, env *Env, _ string) {
				env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
					return AgyProbeResult{Installed: false}, nil
				}
			},
			wantState:  NotSignedIn,
			wantReason: "not installed",
		},
		{
			name: "corrupt oauth token file reports unknown without leaking bytes",
			setup: func(t *testing.T, _ *Env, dataDir string) {
				writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), `{"token":`+fixtureAgyAccess, testNow)
			},
			wantState:  Unknown,
			wantReason: "unreadable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testEnv(t)
			dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
			tc.setup(t, &env, dataDir)

			d, err := ReadOne(context.Background(), env, "agy")
			if err != nil {
				t.Fatalf("ReadOne: %v", err)
			}
			s := d.Subscription
			if s.State != tc.wantState {
				t.Fatalf("state = %q, want %q (reason %q)", s.State, tc.wantState, s.StateReason)
			}
			gotPlan := ""
			if s.Plan != nil {
				gotPlan = *s.Plan
			}
			if gotPlan != tc.wantPlan {
				t.Fatalf("plan = %q, want %q (reason %q)", gotPlan, tc.wantPlan, s.PlanReason)
			}
			if tc.wantReason != "" && !strings.Contains(strings.ToLower(s.StateReason), strings.ToLower(tc.wantReason)) {
				t.Fatalf("state_reason = %q, want substring %q", s.StateReason, tc.wantReason)
			}
			if (s.Weekly != nil) != tc.wantWeekly {
				t.Fatalf("weekly present = %v, want %v (%+v)", s.Weekly != nil, tc.wantWeekly, s.Weekly)
			}
			hasSession := false
			for _, m := range s.Meters {
				// Honest limits rule: agy never fabricates a percentage!
				if m.Percent != nil {
					t.Fatalf("agy meter %q fabricated percent %v", m.Label, *m.Percent)
				}
				if m.PercentReason == "" {
					t.Fatalf("agy meter %q missing percent_reason", m.Label)
				}
				if m.Kind == KindSession {
					hasSession = true
				}
			}
			if hasSession != tc.wantSession {
				t.Fatalf("session meter = %v, want %v (meters %+v)", hasSession, tc.wantSession, s.Meters)
			}
		})
	}
}

func TestAgyPlanAnnotationsFromSubscriptionsTomlAndHostConfig(t *testing.T) {
	t.Run("subscriptions.toml [[plans]] plan=ultra", func(t *testing.T) {
		env := testEnv(t)
		dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
		writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
		plansFile := filepath.Join(env.Home, "subscriptions.toml")
		writeFile(t, plansFile, "[[plans]]\nname = 'google-ai-ultra'\nharness = 'agy'\nplan = 'ultra'\nmonthly_usd = 250\nreset_day = 1\n", testNow)
		plans, err := LoadPlans(plansFile)
		if err != nil {
			t.Fatal(err)
		}
		env.Plans = plans
		d, err := ReadOne(context.Background(), env, "agy")
		if err != nil {
			t.Fatal(err)
		}
		if d.Subscription.Plan == nil || *d.Subscription.Plan != "ultra" {
			t.Fatalf("plan = %v, want ultra", d.Subscription.Plan)
		}
		if strings.Join(d.Subscription.ConfiguredPlans, ",") != "google-ai-ultra" {
			t.Fatalf("configured_plans = %v", d.Subscription.ConfiguredPlans)
		}
	})

	t.Run("subscriptions.toml [agy] plan=pro without spend entry", func(t *testing.T) {
		env := testEnv(t)
		dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
		writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
		plansFile := filepath.Join(env.Home, "subscriptions.toml")
		writeFile(t, plansFile, "[agy]\nplan = 'Google AI Pro'\n", testNow)
		plans, err := LoadPlans(plansFile)
		if err != nil {
			t.Fatal(err)
		}
		env.Plans = plans
		d, _ := ReadOne(context.Background(), env, "agy")
		if d.Subscription.Plan == nil || *d.Subscription.Plan != "pro" || len(d.Subscription.ConfiguredPlans) != 0 {
			t.Fatalf("got plan=%v configured=%v", d.Subscription.Plan, d.Subscription.ConfiguredPlans)
		}
	})

	t.Run("dispatch.toml [harnesses.agy] plan=ultra", func(t *testing.T) {
		env := testEnv(t)
		dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
		writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
		writeFile(t, filepath.Join(env.Home, ".config", "hev", "dispatch.toml"), "[harnesses.agy]\nplan = 'ultra'\n", testNow)
		d, _ := ReadOne(context.Background(), env, "agy")
		if d.Subscription.Plan == nil || *d.Subscription.Plan != "ultra" {
			t.Fatalf("got plan=%v", d.Subscription.Plan)
		}
	})

	t.Run("plan annotation never masks api-key billing", func(t *testing.T) {
		env := testEnv(t)
		dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
		writeFile(t, filepath.Join(dataDir, "settings.json"), `{"modelProvider":"gemini"}`, testNow)
		env.Plans = []ConfiguredPlan{{Name: "google-ai-ultra", Harness: "agy", Tier: "ultra"}}
		d, _ := ReadOne(context.Background(), env, "agy")
		if d.Subscription.State != NotSignedIn || d.Subscription.Plan == nil || *d.Subscription.Plan != "api-key" {
			t.Fatalf("api-key masked by plan annotation: %+v", d.Subscription)
		}
	})
}

func TestAgyLocalActivityMetricsAndHonestLimits(t *testing.T) {
	env := testEnv(t)
	dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
	writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)

	// Session 1 (1 hour ago, inside both 5h and 7d): prefers transcript_full.jsonl over transcript.jsonl, skips RUNNING step.
	writeAgyStepTranscript(t, dataDir, "conv-recent", "transcript.jsonl", testNow.Add(-time.Hour),
		`{"step_index":0,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2030-03-06T11:00:00Z","model":"gemini-4-carbon-high","input_tokens":999999}`)
	writeAgyStepTranscript(t, dataDir, "conv-recent", "transcript_full.jsonl", testNow.Add(-time.Hour),
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2030-03-06T10:55:00Z","content":"hello"}`,
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2030-03-06T11:00:00Z","model":"gemini-4-carbon-high","input_tokens":10000,"output_tokens":1500,"cache_read_tokens":20000}`,
		`{"step_index":2,"type":"PLANNER_RESPONSE","status":"RUNNING","created_at":"2030-03-06T11:05:00Z","model":"gemini-4-carbon-high","input_tokens":500000}`)

	// Session 2 (2 days ago, inside 7d only).
	writeAgyStepTranscript(t, dataDir, "conv-2d", "transcript_full.jsonl", testNow.Add(-48*time.Hour),
		`{"step_index":0,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2030-03-04T12:00:00Z","model":"gemini-4-neon-medium","input_tokens":5000,"output_tokens":500,"cache_read_tokens":4500}`)

	// Session 3 (10 days ago, outside 7d): must be ignored.
	writeAgyStepTranscript(t, dataDir, "conv-old", "transcript_full.jsonl", testNow.Add(-10*24*time.Hour),
		`{"step_index":0,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2030-02-24T12:00:00Z","model":"gemini-4-carbon-high","input_tokens":800000}`)

	// Session 4 present only in conversation_summaries.db (30m ago).
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "conversation_summaries.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE conversation_summaries (conversation_id TEXT PRIMARY KEY, step_count INTEGER NOT NULL DEFAULT 0, last_modified_time DATETIME NOT NULL);
		INSERT INTO conversation_summaries VALUES ('conv-db-only', 4, '2030-03-06T11:30:00Z');
		INSERT INTO conversation_summaries VALUES ('conv-recent', 2, '2030-03-06T11:00:00Z');`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	d, err := ReadOne(context.Background(), env, "agy")
	if err != nil {
		t.Fatal(err)
	}
	s := d.Subscription
	// Passive Carbon usage in transcript infers Ultra even without running `agy models`.
	if s.State != SignedIn || s.Plan == nil || *s.Plan != "ultra" {
		t.Fatalf("state=%s plan=%v", s.State, s.Plan)
	}
	if s.Activity == nil || s.Activity.Session == nil || s.Activity.Weekly == nil {
		t.Fatalf("missing activity: %+v", s.Activity)
	}
	// 5h window: conv-recent + conv-db-only = 2 sessions, 1 planner turn, 31500 total tokens.
	if got := *s.Activity.Session; got.Sessions != 2 || got.Turns != 1 || got.InputTokens != 10000 || got.OutputTokens != 1500 || got.CacheReadTokens != 20000 || got.TotalTokens != 31500 {
		t.Fatalf("session activity = %+v", got)
	}
	// 7d window: conv-recent + conv-2d + conv-db-only = 3 sessions, 2 planner turns, 41500 total tokens.
	if got := *s.Activity.Weekly; got.Sessions != 3 || got.Turns != 2 || got.TotalTokens != 41500 {
		t.Fatalf("weekly activity = %+v", got)
	}
	if s.ObservedAt == nil || !s.ObservedAt.Equal(testNow.Add(-30*time.Minute)) {
		t.Fatalf("observed_at = %v, want 11:30Z", s.ObservedAt)
	}

	// Render list and detail: WEEKLY must be `?` (never `0%`), and detail must include activity note.
	var listOut, detailOut bytes.Buffer
	if err := WriteList(&listOut, List{GeneratedAt: testNow, Subscriptions: []Status{s}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOut.String(), "ultra") || !strings.Contains(listOut.String(), "?") || strings.Contains(listOut.String(), "0%") {
		t.Fatalf("unexpected list output:\n%s", listOut.String())
	}
	if err := WriteDetail(&detailOut, d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detailOut.String(), "3 sessions, 2 turns, 41.5k tokens") {
		t.Fatalf("detail missing weekly activity summary:\n%s", detailOut.String())
	}

	// Passive Flash-only transcript does NOT falsely downgrade an account to "standard";
	// without `agy models` or annotation, Plan stays nil ("unknown").
	envFlash := testEnv(t)
	flashDir := filepath.Join(envFlash.Home, ".gemini", "antigravity-cli")
	writeFile(t, filepath.Join(flashDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
	writeAgyStepTranscript(t, flashDir, "c1", "transcript_full.jsonl", testNow.Add(-time.Minute),
		`{"step_index":0,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2030-03-06T11:59:00Z","model":"gemini-3.8-flash-high","input_tokens":100,"output_tokens":20}`)
	dFlash, _ := ReadOne(context.Background(), envFlash, "agy")
	if dFlash.Subscription.Plan != nil || dFlash.Subscription.PlanReason == "" {
		t.Fatalf("flash-only transcript guessed plan %v", dFlash.Subscription.Plan)
	}
}

func TestAgyProbeTimeoutCacheAndBounds(t *testing.T) {
	t.Run("caching and rate-limit interval", func(t *testing.T) {
		env := testEnv(t)
		dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
		writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
		var calls atomic.Int32
		env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
			calls.Add(1)
			return AgyProbeResult{Installed: true, Models: []string{"gemini-4-carbon-high", "gemini-4-neon-medium"}}, nil
		}

		first, _ := ReadOne(context.Background(), env, "agy")
		env.Now = func() time.Time { return testNow.Add(DefaultMinInterval - time.Second) }
		second, _ := ReadOne(context.Background(), env, "agy")
		if calls.Load() != 1 {
			t.Fatalf("two reads inside interval made %d probe calls", calls.Load())
		}
		if second.Subscription.Plan == nil || *second.Subscription.Plan != "ultra" || !strings.Contains(second.Subscription.Source, "(cached)") {
			t.Fatalf("second = %+v, first = %+v", second.Subscription, first.Subscription)
		}

		// Past the interval boundary, a fresh probe is permitted.
		env.Now = func() time.Time { return testNow.Add(DefaultMinInterval) }
		_, _ = ReadOne(context.Background(), env, "agy")
		if calls.Load() != 2 {
			t.Fatalf("expected 2 probe calls at boundary, got %d", calls.Load())
		}
	})

	t.Run("subprocess timeout preserves cached tier and backs off", func(t *testing.T) {
		env := testEnv(t)
		env.AgyProbeTimeout = 40 * time.Millisecond
		dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
		writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)

		var calls atomic.Int32
		hang := false
		env.AgyProbe = func(ctx context.Context) (AgyProbeResult, error) {
			calls.Add(1)
			if hang {
				<-ctx.Done()
				return AgyProbeResult{Installed: true}, ctx.Err()
			}
			return AgyProbeResult{Installed: true, Models: []string{"gemini-4-carbon-high"}}, nil
		}

		// Seed good cache.
		_, _ = ReadOne(context.Background(), env, "agy")
		// Advance past interval and make the probe hang.
		later := testNow.Add(DefaultMinInterval + time.Minute)
		env.Now = func() time.Time { return later }
		hang = true

		start := time.Now()
		d, err := ReadOne(context.Background(), env, "agy")
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("probe timeout did not bound read: took %s", elapsed)
		}
		s := d.Subscription
		if !s.Stale || !strings.Contains(s.StaleReason, "timed out") {
			t.Fatalf("expected stale timeout reason, got stale=%v reason=%q", s.Stale, s.StaleReason)
		}
		if s.Plan == nil || *s.Plan != "ultra" {
			t.Fatalf("cached plan lost on timeout: %v", s.Plan)
		}
		// Another read inside the interval must NOT re-run the hanging probe.
		_, _ = ReadOne(context.Background(), env, "agy")
		if calls.Load() != 2 {
			t.Fatalf("timeout was not rate-limited: calls = %d", calls.Load())
		}
	})

	t.Run("concurrent lock busy skips probe cleanly", func(t *testing.T) {
		env := testEnv(t)
		dataDir := filepath.Join(env.Home, ".gemini", "antigravity-cli")
		writeFile(t, filepath.Join(dataDir, "antigravity-oauth-token"), agyTokenJSON(testNow.Add(time.Hour), true, "consumer"), testNow)
		var calls atomic.Int32
		env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
			calls.Add(1)
			return AgyProbeResult{Installed: true}, nil
		}
		path := AgyCachePath(env)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		unlock, err := platformops.LockPath(path+".lock", true)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()

		d, _ := ReadOne(context.Background(), env, "agy")
		if calls.Load() != 0 || !d.Subscription.Stale || !strings.Contains(d.Subscription.StaleReason, "another process") {
			t.Fatalf("calls=%d status=%+v", calls.Load(), d.Subscription)
		}
	})

	t.Run("transcript file count cap and spinner parsing", func(t *testing.T) {
		rawOut := "\r⠋ Fetching available models...\r\x1b[2Kgemini-4-carbon-high   Gemini 4 Carbon (High)\n" +
			"ya29.should-not-be-a-model   Ignore\n" +
			"gemini-4-neon-medium   Gemini 4 Neon (Medium)\n"
		res, err := parseAgyProbeOutput(rawOut, "", nil)
		if err != nil || len(res.Models) != 2 || res.Models[0] != "gemini-4-carbon-high" {
			t.Fatalf("parsed = %+v, err = %v", res, err)
		}
		if !hasWindowsCredentialTarget("Currently stored credentials:\r\n    Target: LegacyGeneric:target=antigravity-oauth-token:default\r\n", "antigravity-oauth-token") {
			t.Fatal("expected Windows credential target match")
		}
		if hasWindowsCredentialTarget("Currently stored credentials:\r\n * NONE *\r\n", "antigravity-oauth-token") {
			t.Fatal("unexpected Windows credential target match on NONE")
		}
	})
}

func TestAgyRedactsSecretsTokensEmailsAndLocalPaths(t *testing.T) {
	env := testEnv(t)
	secretVaultDir := filepath.Join(env.Home, "private-secret-vault", ".gemini", "antigravity-cli")
	jwtToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NSJ9.signatureBlock999"
	winSecretPath := `C:\Users\secret-op\.gemini\antigravity-cli\antigravity-oauth-token`

	env.Getenv = func(k string) string {
		if k == "HEV_AGY_DATA_DIR" {
			return secretVaultDir
		}
		return ""
	}
	writeFile(t, filepath.Join(secretVaultDir, "antigravity-oauth-token"),
		fmt.Sprintf(`{"token":{"access_token":%q,"refresh_token":%q,"id_token":%q,"expiry":%q},"auth_method":"consumer","id_token_email":%q}`,
			fixtureAgyAccess, fixtureAgyRefresh, jwtToken, testNow.Add(time.Hour).Format(time.RFC3339), fixtureAgyEmail), testNow)

	stepContent, _ := json.Marshal("token " + fixtureAgyAccess + " " + fixtureAgyAPIKey + " " + winSecretPath)
	writeAgyStepTranscript(t, secretVaultDir, "conv-sec", "transcript_full.jsonl", testNow.Add(-5*time.Minute),
		fmt.Sprintf(`{"step_index":0,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2030-03-06T11:55:00Z","model":"gemini-4-carbon-high","input_tokens":2000,"output_tokens":300,"content":%s}`,
			string(stepContent)))

	// Probe returns an error carrying raw credentials and paths; nothing in it may escape.
	env.AgyProbe = func(context.Context) (AgyProbeResult, error) {
		return AgyProbeResult{
			Installed: true,
			Models:    []string{"gemini-4-carbon-high", fixtureAgyAccess, fixtureAgyAPIKey},
		}, errors.New("probe failed for " + fixtureAgyEmail + " Bearer " + fixtureAgyAccess + " key=" + fixtureAgyAPIKey + " ref=" + fixtureAgySecret + " at " + secretVaultDir + " and " + winSecretPath)
	}

	d, err := ReadOne(context.Background(), env, "agy")
	if err != nil {
		t.Fatal(err)
	}
	all := ReadAll(context.Background(), env)

	var textBuf bytes.Buffer
	_ = WriteList(&textBuf, all)
	_ = WriteDetail(&textBuf, d)
	jsonBytes, _ := json.Marshal(d)
	cacheBytes, err := os.ReadFile(AgyCachePath(env))
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(AgyCachePath(env)); err == nil && os.PathSeparator == '/' && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("agy cache mode %v is world/group accessible", info.Mode())
	}

	forbidden := []string{
		fixtureAgyAccess,
		fixtureAgyRefresh,
		fixtureAgyAPIKey,
		fixtureAgyEmail,
		fixtureAgySecret,
		jwtToken,
		secretVaultDir,
		filepath.ToSlash(secretVaultDir),
		winSecretPath,
		`C:\Users\secret-op`,
		"private-secret-vault",
		"antigravity-oauth-token",
	}
	surfaces := map[string]string{
		"rendered text": textBuf.String(),
		"detail JSON":   string(jsonBytes),
		"agy.json":      string(cacheBytes),
	}
	for surface, content := range surfaces {
		for _, secret := range forbidden {
			if strings.Contains(content, secret) {
				t.Fatalf("%s leaked %q:\n%s", surface, secret, content)
			}
		}
	}

	// Positive side of the rule: safe metadata (plan tier and token counts) is kept!
	if d.Subscription.Plan == nil || *d.Subscription.Plan != "ultra" {
		t.Fatalf("safe plan tier dropped: %v", d.Subscription.Plan)
	}
	if !strings.Contains(textBuf.String(), "2.3k tokens") {
		t.Fatalf("safe token count dropped:\n%s", textBuf.String())
	}

	// Direct unit test for redactAgyText on every secret class.
	dirty := strings.Join([]string{
		"tok=" + fixtureAgyAccess,
		"ref=" + fixtureAgyRefresh,
		"key=" + fixtureAgyAPIKey,
		"jwt=" + jwtToken,
		"op=" + fixtureAgySecret,
		"mail=" + fixtureAgyEmail,
		"unix=/Users/alice/.gemini/antigravity-cli/antigravity-oauth-token",
		"win=" + winSecretPath,
	}, " ")
	scrubbed := redactAgyText(dirty, secretVaultDir)
	for _, secret := range forbidden {
		if strings.Contains(scrubbed, secret) {
			t.Fatalf("redactAgyText left %q in %q", secret, scrubbed)
		}
	}
}
