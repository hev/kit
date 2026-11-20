package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// checkPlainLaunch requires a non-agy attempt to carry neither agy's
// resolved env_refs nor its --settings flag.
func checkPlainLaunch(t *testing.T, where string, args, env []string) {
	t.Helper()
	if hasEnv(env, "GEMINI_API_KEY") {
		t.Fatalf("%s: non-agy harness received agy's env_refs", where)
	}
	// claude passes its own hook --settings; only agy's provider is refused.
	if slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "modelProvider") }) {
		t.Fatalf("%s: non-agy harness received agy's --settings: %q", where, args)
	}
}

// fallbackLaunchFixture puts fake agy and claude harnesses on PATH and a
// dispatch.toml with [harnesses.agy].env_refs in place, returning the dump
// directory both fakes record into.
func fallbackLaunchFixture(t *testing.T) (cfg, dump string) {
	t.Helper()
	orchHome(t)
	allowChildProbes(t)
	fakeHarnessSecrets(t, testGeminiSecret, nil)
	cfg = writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiRef+"\"\n")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	_ = os.Unsetenv("GEMINI_API_KEY")
	bin := t.TempDir()
	dump = t.TempDir()
	installEnvDump(t, bin, "agy")
	installEnvDump(t, bin, "claude")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISPATCH_ENV_DUMP", dump)
	t.Setenv("DISPATCH_ECHO_ENV", "GEMINI_API_KEY")
	return cfg, dump
}

// Every attempt of a dispatch run's fallback chain is launched for its own
// harness: agy gets its env_refs, --settings and redacted output whether it
// is the primary or the fallback, and a non-agy attempt gets neither.
func TestWorkerFallbackAppliesHarnessEnvPerAttempt(t *testing.T) {
	for _, tc := range []struct {
		name            string
		primary, backup string
	}{
		{"agy-then-claude", "agy", "claude"},
		{"claude-then-agy", "claude", "agy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, dump := fallbackLaunchFixture(t)
			t.Setenv("DISPATCH_FAIL_AS", tc.primary)
			name := "fallback-" + tc.name
			dir := filepath.Join(Root(), name)
			if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
				t.Fatal(err)
			}
			m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true,
				LaunchEnv: []string{"DISPATCH_ENV_DUMP", "DISPATCH_ECHO_ENV", "DISPATCH_FAIL_AS", "PATH"},
				Order: Order{Name: name, Repo: "app", Harness: tc.primary, Timeout: 1,
					Fallbacks: FallbackChain{{Harness: tc.backup}}}}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
				t.Fatal(err)
			}
			_ = superviseRound(name)
			var r Result
			if err := readJSON(filepath.Join(dir, "result.json"), &r); err != nil {
				t.Fatal(err)
			}
			if len(r.Attempts) != 2 || r.Attempts[0].Harness != tc.primary || r.Attempts[1].Harness != tc.backup || r.ExitCode != 0 {
				t.Fatalf("attempts %+v exit %d error %q", r.Attempts, r.ExitCode, r.Error)
			}
			checkAgyLaunch(t, "agy attempt", readArgsDump(t, dump, "agy"), readEnvDump(t, dump, "agy"))
			checkPlainLaunch(t, "claude attempt", readArgsDump(t, dump, "claude"), readEnvDump(t, dump, "claude"))
			checkRedacted(t, filepath.Join(dir, "log"))
			assertNoSecretOnDisk(t, dir, testGeminiSecret)
		})
	}
}

// An orchestrator round's fallback chain does the same: the agy attempt is
// launched with its env_refs and --settings, the claude fallback without.
func TestOrchestratorFallbackAppliesHarnessEnvPerAttempt(t *testing.T) {
	_, dump := fallbackLaunchFixture(t)
	t.Setenv("DISPATCH_FAIL_AS", "agy")
	s := applyOrch(t)
	s.Harness, s.Model, s.Effort = "agy", "", ""
	s.Fallbacks = FallbackChain{{Harness: "claude"}}
	round := t.TempDir()
	st := &OrchestratorState{}
	d := OrchestratorDriver{Session: func(ctx context.Context, s OrchestratorSpec, _ string, p OrchestratorPacket) error {
		// Each attempt runs the real launch in the round directory.
		return orchestratorSession(ctx, s, round, p)
	}}
	// Neither fake writes checkpoint.md, so both attempts fail; only the
	// launches matter here.
	if err := runOrchestratorCandidates(context.Background(), s, round, OrchestratorPacket{}, st, d); err == nil {
		t.Fatal("expected the round to fail without a checkpoint")
	}
	if len(st.Attempts) != 2 || st.Attempts[0].Harness != "agy" || st.Attempts[1].Harness != "claude" {
		t.Fatalf("attempts %+v", st.Attempts)
	}
	checkAgyLaunch(t, "agy round attempt", readArgsDump(t, dump, "agy"), readEnvDump(t, dump, "agy"))
	checkPlainLaunch(t, "claude round attempt", readArgsDump(t, dump, "claude"), readEnvDump(t, dump, "claude"))
	// The agy attempt's trace was set aside when the fallback started.
	checkRedacted(t, filepath.Join(round, "trace.attempt-1"))
	assertNoSecretOnDisk(t, round, testGeminiSecret)
	assertNoSecretOnDisk(t, OrchestratorDir(s.Name), testGeminiSecret)
}
