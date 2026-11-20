package dispatch

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func degradeFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "degrade", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The incident's own records: Claude's stream-json and transcript spell the
// error fields differently, and both are a usage limit with its window.
func TestClassifyLimitClaudeCaptured(t *testing.T) {
	reset := time.Unix(1795266000, 0).UTC()
	for _, file := range []string{"claude-stream-usage-limit.jsonl", "claude-transcript-usage-limit.jsonl"} {
		hit := ClassifyLimit("claude", degradeFixture(t, file))
		if hit == nil {
			t.Fatalf("%s: no limit", file)
		}
		if hit.Kind != "usage_limit_reached" || hit.LimitType != "seven_day_overage_included" || !hit.ResetsAt.Equal(reset) || !strings.HasPrefix(hit.Message, "You've hit your Fable limit") {
			t.Fatalf("%s: %+v", file, hit)
		}
	}
}

// A 429 rate_limit_error whose rate_limit_info is rejected is a limit even
// without apiError, and so is the bare limit message on an error record.
func TestClassifyLimitClaudeVariants(t *testing.T) {
	record := string(degradeFixture(t, "claude-transcript-usage-limit.jsonl"))
	rejected := strings.Replace(record, `"apiError":"usage_limit_reached",`, "", 1)
	if rejected == record {
		t.Fatal("fixture shape changed")
	}
	hit := ClassifyLimit("claude", []byte(rejected))
	if hit == nil || hit.Kind != "rate_limit_rejected" || hit.LimitType != "seven_day_overage_included" {
		t.Fatalf("429 rejected: %+v", hit)
	}
	allowed := strings.Replace(rejected, `"rate_limit_info":{"status":"rejected"`, `"rate_limit_info":{"status":"allowed"`, 1)
	allowed = strings.Replace(allowed, "You've hit your Fable limit", "Rate limited", 1)
	if hit := ClassifyLimit("claude", []byte(allowed)); hit != nil {
		t.Fatalf("429 allowed is not a limit: %+v", hit)
	}
	message := `{"type":"result","subtype":"success","is_error":true,"result":"You've hit your session limit · resets 9:40am (America/Denver)"}`
	if hit := ClassifyLimit("claude", []byte(message)); hit == nil || hit.Kind != "limit_message" {
		t.Fatalf("limit message: %+v", hit)
	}
	// The same text from the model, not an error record, is not a limit.
	quoted := `{"type":"result","subtype":"success","is_error":false,"result":"You've hit your session limit · resets 9:40am"}`
	if hit := ClassifyLimit("claude", []byte(quoted)); hit != nil {
		t.Fatalf("quoted message: %+v", hit)
	}
}

func TestClassifyLimitCodexCaptured(t *testing.T) {
	want := time.Date(2026, 9, 19, 2, 12, 0, 0, time.Local).UTC()
	for _, file := range []string{"codex-rollout-usage-limit.jsonl", "codex-exec-usage-limit.jsonl"} {
		hit := ClassifyLimit("codex", degradeFixture(t, file))
		if hit == nil || hit.Kind != "usage_limit_exceeded" || !hit.ResetsAt.Equal(want) || !strings.HasPrefix(hit.Message, "You've hit your usage limit") {
			t.Fatalf("%s: %+v", file, hit)
		}
	}
}

// A crash, a failed test, a 529 overload, a policy refusal and a capacity
// error are ordinary failures; a timeout or a stop is never a limit even
// when the log holds one.
func TestClassifyLimitNegatives(t *testing.T) {
	for _, c := range []struct{ harness, file string }{
		{"claude", "claude-crash.jsonl"},
		{"claude", "claude-test-failure.jsonl"},
		{"claude", "claude-overloaded-529.jsonl"},
		{"codex", "codex-exec-cyber-policy.jsonl"},
		{"codex", "codex-exec-capacity.jsonl"},
	} {
		if hit := ClassifyLimit(c.harness, degradeFixture(t, c.file)); hit != nil {
			t.Fatalf("%s: %+v", c.file, hit)
		}
		if hit := limitFromRun(c.harness, degradeFixture(t, c.file), 1, false, false, ""); hit != nil {
			t.Fatalf("%s run: %+v", c.file, hit)
		}
	}
	limit := degradeFixture(t, "claude-stream-usage-limit.jsonl")
	if hit := limitFromRun("claude", limit, 1, true, false, ""); hit != nil {
		t.Fatalf("timeout: %+v", hit)
	}
	if hit := limitFromRun("claude", limit, 143, false, true, ""); hit != nil {
		t.Fatalf("stopped: %+v", hit)
	}
	if hit := limitFromRun("claude", limit, 1, false, false, ""); hit == nil {
		t.Fatal("limit exit not classified")
	}
	// Each harness reads only its own records.
	if hit := ClassifyLimit("codex", limit); hit != nil {
		t.Fatalf("claude record as codex: %+v", hit)
	}
}

// codex exec does not carry codex_error_info; the session's rollout does.
func TestLimitFromRunReadsCodexRollout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	session := "01a0a13c-0000-7000-8000-000000000000"
	day := filepath.Join(home, "sessions", "2026", "09", "14")
	if err := os.MkdirAll(day, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(day, "rollout-2026-09-14T18-50-41-"+session+".jsonl"), degradeFixture(t, "codex-rollout-usage-limit.jsonl"), 0600); err != nil {
		t.Fatal(err)
	}
	log := []byte(`{"type":"thread.started","thread_id":"` + session + `"}` + "\n" + `{"type":"turn.failed","error":{"message":"stream ended"}}` + "\n")
	if hit := limitFromRun("codex", log, 1, false, false, session); hit == nil || hit.Kind != "usage_limit_exceeded" {
		t.Fatalf("rollout: %+v", hit)
	}
	if hit := limitFromRun("codex", log, 0, false, false, session); hit != nil {
		t.Fatalf("a clean exit does not read the rollout: %+v", hit)
	}
}

func steps(t *testing.T, entries ...string) []ModelStep {
	t.Helper()
	var out []ModelStep
	for _, e := range entries {
		s, err := ParseModelStep(e)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestDegradeChainDefaults(t *testing.T) {
	fable := ModelStep{Harness: "claude", Model: "claude-fable-5-2"}
	opus := ModelStep{Harness: "claude", Model: "claude-opus-5-5"}
	for _, c := range []struct {
		start ModelStep
		want  []ModelStep
	}{
		{fable, steps(t, "claude/claude-fable-5-2", "claude/claude-opus-5-5", "claude/claude-sonnet-5-5", "codex")},
		{opus, steps(t, "claude/claude-opus-5-5", "claude/claude-sonnet-5-5", "codex")},
		// The end of the chain has nowhere to go.
		{ModelStep{Harness: "codex"}, steps(t, "codex")},
		// A model outside the chain is followed by the whole chain.
		{ModelStep{Harness: "codex", Model: "gpt-6"}, steps(t, "codex/gpt-6", "claude/claude-fable-5-2", "claude/claude-opus-5-5", "claude/claude-sonnet-5-5", "codex")},
	} {
		if got := DegradeChain(c.start, "", Config{}, ""); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%v: %v, want %v", c.start, got, c.want)
		}
	}
}

func TestDegradeChainOverrides(t *testing.T) {
	opus := ModelStep{Harness: "claude", Model: "claude-opus-5-5"}
	cfg := Config{
		Degrade: DegradeConfig{Chain: []string{"claude/claude-opus-5-5", "codex/gpt-6"}},
		Hosts: map[string]Host{
			"mini": {Local: true, Degrade: &DegradeConfig{Chain: []string{"claude/claude-opus-5-5", "claude/claude-sonnet-5-5"}}},
			"off":  {Local: true, Degrade: &DegradeConfig{Off: true}},
			"bare": {Local: true},
		},
	}
	for _, c := range []struct {
		host, order string
		want        []ModelStep
	}{
		{"bare", "", steps(t, "claude/claude-opus-5-5", "codex/gpt-6")},
		{"mini", "", steps(t, "claude/claude-opus-5-5", "claude/claude-sonnet-5-5")},
		{"off", "", steps(t, "claude/claude-opus-5-5")},
		{"mini", "claude/claude-opus-5-5,codex", steps(t, "claude/claude-opus-5-5", "codex")},
		{"mini", "[claude-opus-5-5, codex]", steps(t, "claude/claude-opus-5-5", "codex")},
		{"mini", DegradeOff, steps(t, "claude/claude-opus-5-5")},
	} {
		if got := DegradeChain(opus, c.order, cfg, c.host); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("host %s order %q: %v, want %v", c.host, c.order, got, c.want)
		}
	}
}

func TestOrderDegradeKey(t *testing.T) {
	order := func(v string) string {
		return "---\nname: pinned\nrepo: app\nharness: claude\nmodel: claude-opus-5-5\ndegrade: " + v + "\n---\n## Acceptance\n- done\n"
	}
	for v, want := range map[string]string{
		"off":                               DegradeOff,
		"on":                                "",
		"[claude/claude-sonnet-5-5, codex]": "claude/claude-sonnet-5-5,codex",
		"codex/gpt-6":                       "codex/gpt-6",
	} {
		o, err := Parse(order(v))
		if err != nil || o.Degrade != want {
			t.Fatalf("%q: %q %v", v, o.Degrade, err)
		}
	}
	for _, bad := range []string{"[gemini/x]", "[]", "claude/"} {
		if _, err := Parse(order(bad)); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestDegradeConfigLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dispatch.toml")
	text := `
[degrade]
chain = ["claude/claude-fable-5-2", "claude/claude-opus-5-5", "codex"]
on_degrade = ["~/bin/on-degrade"]
hook_timeout = "5s"

[hosts.mini]
ssh = "mini"
[hosts.mini.degrade]
chain = ["claude/claude-opus-5-5", "codex/gpt-6"]
`
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := cfg.degradeFor("mini"); !reflect.DeepEqual(d.Chain, []string{"claude/claude-opus-5-5", "codex/gpt-6"}) || d.HookTimeout != "5s" || len(d.OnDegrade) != 1 {
		t.Fatalf("host merge: %+v", d)
	}
	if d := cfg.degradeFor("oven"); len(d.Chain) != 3 {
		t.Fatalf("global: %+v", d)
	}
	if _, err := loadTarget(path, "mini"); err != nil {
		t.Fatalf("target load: %v", err)
	}
	if err := os.WriteFile(path, []byte("[degrade]\nchain = [\"gemini/x\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "degrade.chain") {
		t.Fatalf("bad chain: %v", err)
	}
}

// A validator never runs on a model its implementer ran, including one the
// implementer degraded to, unless --allow-same-model.
func TestValidatorChainStaysIndependent(t *testing.T) {
	chain := DegradeChain(ModelStep{Harness: "claude", Model: "claude-opus-5-5"}, "", Config{}, "")
	implementer := steps(t, "claude/claude-fable-5-2", "claude/claude-opus-5-5")
	if got := independentChain(chain, implementer, false); !reflect.DeepEqual(got, steps(t, "claude/claude-sonnet-5-5", "codex")) {
		t.Fatalf("independent: %v", got)
	}
	if got := independentChain(chain, implementer, true); !reflect.DeepEqual(got, chain) {
		t.Fatalf("allow: %v", got)
	}
	// The worker degraded to Sonnet; a Sonnet validator steps on to Codex.
	m := Meta{Parent: "app", Implementer: steps(t, "claude/claude-opus-5-5", "claude/claude-sonnet-5-5"), Order: Order{Harness: "claude", Model: "claude-sonnet-5-5"}}
	if got := runDegradeChain("", m, Config{}, nil); !reflect.DeepEqual(got, steps(t, "codex")) {
		t.Fatalf("validator chain: %v", got)
	}
}

// Fresh observations skip a model; stale ones never do.
func TestKnownExhaustedFreshAndStale(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now().UTC()
	fable := ModelStep{Harness: "claude", Model: "claude-fable-5-2"}
	opus := ModelStep{Harness: "claude", Model: "claude-opus-5-5"}
	recordLimitObservation(fable, &LimitHit{LimitType: "seven_day_overage_included", ResetsAt: now.Add(48 * time.Hour)}, "run", now)
	if o := knownExhausted(DegradeConfig{}, fable, now); o == nil || o.LimitType != "seven_day_overage_included" {
		t.Fatalf("fresh: %+v", o)
	}
	if o := knownExhausted(DegradeConfig{}, opus, now); o != nil {
		t.Fatalf("other model: %+v", o)
	}
	if o := knownExhausted(DegradeConfig{}, fable, now.Add(49*time.Hour)); o != nil {
		t.Fatalf("past its reset: %+v", o)
	}
	// Without a reset time an observation is fresh for an hour.
	recordLimitObservation(opus, &LimitHit{}, "run", now)
	if knownExhausted(DegradeConfig{}, opus, now.Add(30*time.Minute)) == nil || knownExhausted(DegradeConfig{}, opus, now.Add(2*time.Hour)) != nil {
		t.Fatal("reset-less observation window")
	}

	// Plan observations (internal/cost): a fresh 100% weekly reading skips
	// the plan's model; the same reading two hours old does not.
	factory := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(factory, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("prices.toml", "")
	write("subscriptions.toml", "[[plans]]\nname = \"max\"\nharness = \"claude\"\nmodels = [\"claude-sonnet-5-5\"]\nreset_day = 4\n")
	write("sessions.jsonl", "")
	sonnet := ModelStep{Harness: "claude", Model: "claude-sonnet-5-5"}
	usage := func(observed time.Time) string {
		return `{"plan":"max","observed_at":` + strconv.FormatInt(observed.UnixMilli(), 10) + `,"limits":[{"kind":"weekly_all","percent":100,"resets_at":"` + now.Add(24*time.Hour).Format(time.RFC3339) + `"}]}`
	}
	d := DegradeConfig{Observations: factory}
	write("claude-usage.json", usage(now.Add(-time.Minute)))
	if o := knownExhausted(d, sonnet, now); o == nil {
		t.Fatal("fresh plan observation ignored")
	}
	if o := knownExhausted(d, ModelStep{Harness: "claude", Model: "claude-haiku-5-5"}, now); o != nil {
		t.Fatalf("plan does not cover haiku: %+v", o)
	}
	write("claude-usage.json", usage(now.Add(-2*time.Hour)))
	if o := knownExhausted(d, sonnet, now); o != nil {
		t.Fatalf("stale plan observation used: %+v", o)
	}
}
