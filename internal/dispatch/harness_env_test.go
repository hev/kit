package dispatch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets"
)

const (
	testGeminiRef    = "op://example/gemini-key/credential"
	testGeminiSecret = "gemini-secret-value-7f3a91"
	geminiSettings   = `{"modelProvider":"gemini"}`
)

func writeDispatchConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeHarnessSecrets replaces the secrets client; it answers every ref with
// value and records what was asked.
func fakeHarnessSecrets(t *testing.T, value string, err error) *[]secrets.Ref {
	t.Helper()
	var asked []secrets.Ref
	old := resolveHarnessSecrets
	resolveHarnessSecrets = func(_ context.Context, refs []secrets.Ref, caller string) (map[string]string, error) {
		if caller == "" {
			t.Error("secrets resolved without a caller label")
		}
		asked = append(asked, refs...)
		if err != nil {
			return nil, err
		}
		out := map[string]string{}
		for _, r := range refs {
			out[r.Name] = value
		}
		return out, nil
	}
	t.Cleanup(func() { resolveHarnessSecrets = old })
	return &asked
}

func TestHarnessConfigParses(t *testing.T) {
	cfg := writeDispatchConfig(t, `
[hosts.mini]
local = true

[harnesses.agy]
model_provider = "gemini"

[harnesses.agy.env_refs]
GEMINI_API_KEY = "`+testGeminiRef+`"

[harnesses.claude]
env_refs = { ANTHROPIC_API_KEY = "op://example/anthropic/credential" }
`)
	for name, load := range map[string]func() (Config, error){
		"full":   func() (Config, error) { return Load(cfg) },
		"target": func() (Config, error) { return loadTarget(cfg, "mini") },
	} {
		c, err := load()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		agy := c.Harnesses["agy"]
		if agy.ModelProvider != "gemini" || agy.EnvRefs["GEMINI_API_KEY"] != testGeminiRef {
			t.Fatalf("%s: agy settings %+v", name, agy)
		}
		if c.Harnesses["claude"].EnvRefs["ANTHROPIC_API_KEY"] != "op://example/anthropic/credential" {
			t.Fatalf("%s: claude settings %+v", name, c.Harnesses["claude"])
		}
	}
	h, err := harnessSettings(cfg, "agy")
	if err != nil || h.ModelProvider != "gemini" || len(h.EnvRefs) != 1 {
		t.Fatalf("%+v %v", h, err)
	}
	if h, err = harnessSettings(cfg, "codex"); err != nil || h.ModelProvider != "" || len(h.EnvRefs) != 0 {
		t.Fatalf("codex: %+v %v", h, err)
	}
}

func TestHarnessConfigOmittedChangesNothing(t *testing.T) {
	cfg := writeDispatchConfig(t, "[hosts.mini]\nlocal = true\n")
	c, err := Load(cfg)
	if err != nil || c.Harnesses != nil {
		t.Fatalf("%+v %v", c.Harnesses, err)
	}
	asked := fakeHarnessSecrets(t, testGeminiSecret, nil)
	args := harnessArgs("agy", "gemini-4-carbon-high", "", "P")
	env := []string{"PATH=/bin"}
	gotEnv, gotArgs, err := harnessLaunch(context.Background(), cfg, "agy", "test", env, args)
	if err != nil || !slices.Equal(gotEnv, env) || !slices.Equal(gotArgs, args) || len(*asked) != 0 {
		t.Fatalf("%q %q %v asked=%v", gotEnv, gotArgs, err, *asked)
	}
	// A missing config file is the same as no table.
	if _, _, err = harnessLaunch(context.Background(), filepath.Join(t.TempDir(), "none.toml"), "agy", "test", env, args); err != nil {
		t.Fatal(err)
	}
}

func TestHarnessConfigRejectsInvalid(t *testing.T) {
	for body, want := range map[string]string{
		"[harnesses.gpt]\nmodel_provider = \"gemini\"\n":                   "harnesses.gpt",
		"[harnesses.claude]\nmodel_provider = \"gemini\"\n":                "only agy",
		"[harnesses.agy]\nmodel_provider = \"bad provider\"\n":             "invalid provider",
		"[harnesses.agy]\nmodel = \"x\"\n":                                 "unknown config key",
		"[harnesses.agy.env_refs]\n\"1BAD\" = \"" + testGeminiRef + "\"\n": "invalid environment name",
		"[harnesses.agy.env_refs]\nGEMINI_API_KEY = \"plain-value\"\n":     "not a secret reference",
	} {
		cfg := writeDispatchConfig(t, body)
		if _, err := Load(cfg); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Load %q: %v, want %q", body, err, want)
		}
		if _, err := harnessSettings(cfg, "agy"); err == nil {
			t.Fatalf("harnessSettings accepted %q", body)
		}
	}
	// A rejected value never echoes the would-be secret.
	cfg := writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiSecret+"\"\n")
	if _, err := Load(cfg); err == nil || strings.Contains(err.Error(), testGeminiSecret) {
		t.Fatalf("%v", err)
	}
}

func TestAgyModelProviderArgs(t *testing.T) {
	base := harnessArgs("agy", "gemini-4-neon-medium", "high", "do the work")
	cases := []struct {
		name    string
		harness string
		h       HarnessConfig
		env     []string
		want    string
	}{
		{"gemini key present", "agy", HarnessConfig{}, []string{"GEMINI_API_KEY=k"}, geminiSettings},
		{"configured provider", "agy", HarnessConfig{ModelProvider: "gemini"}, nil, geminiSettings},
		{"configured provider wins", "agy", HarnessConfig{ModelProvider: "vertex"}, []string{"GEMINI_API_KEY=k"}, `{"modelProvider":"vertex"}`},
		{"no key, no provider", "agy", HarnessConfig{}, []string{"PATH=/bin"}, ""},
		{"empty key", "agy", HarnessConfig{}, []string{"GEMINI_API_KEY="}, ""},
		{"key cleared later", "agy", HarnessConfig{}, []string{"GEMINI_API_KEY=k", "GEMINI_API_KEY="}, ""},
		{"other harness", "claude", HarnessConfig{}, []string{"GEMINI_API_KEY=k"}, ""},
	}
	for _, c := range cases {
		args := base
		if c.harness != "agy" {
			args = harnessArgs(c.harness, "", "", "do the work")
		}
		got := withModelProvider(args, harnessModelProvider(c.harness, c.h, c.env))
		i := slices.Index(got, "--settings")
		if c.want == "" {
			if i >= 0 || !slices.Equal(got, args) {
				t.Fatalf("%s: %q", c.name, got)
			}
			continue
		}
		// --settings JSON sits immediately before --print=PROMPT, which stays last.
		if i != len(got)-3 || got[i+1] != c.want || got[len(got)-1] != "--print=do the work" {
			t.Fatalf("%s: %q", c.name, got)
		}
		if !slices.Equal(got[:i], args[:len(args)-1]) {
			t.Fatalf("%s: earlier flags changed: %q", c.name, got)
		}
	}
	// A prompt that itself looks like flags is still one --print= value.
	got := withModelProvider(harnessArgs("agy", "", "", "--settings {} --model x"), "gemini")
	want := []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "--settings", geminiSettings, "--print=--settings {} --model x"}
	if !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
	// wireHooks leaves agy's argv alone, so the order survives it.
	wired, _ := wireHooks("agy", filepath.Join(t.TempDir(), "steer"), "dispatch:x", got, nil)
	if !slices.Equal(wired, want) {
		t.Fatalf("%q", wired)
	}
}

func TestHarnessLaunchResolvesEnvRefs(t *testing.T) {
	cfg := writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiRef+"\"\nOTHER_TOKEN = \"op://example/other/credential\"\n")
	asked := fakeHarnessSecrets(t, testGeminiSecret, nil)
	env, args, err := harnessLaunch(context.Background(), cfg, "agy", "test", []string{"PATH=/bin", "GEMINI_API_KEY=inherited"}, harnessArgs("agy", "gemini-4-carbon-high", "", "P"))
	if err != nil {
		t.Fatal(err)
	}
	// Resolved values come last, so they win over an inherited name.
	if envValue(env, "GEMINI_API_KEY") != testGeminiSecret || envValue(env, "OTHER_TOKEN") != testGeminiSecret || envValue(env, "PATH") != "/bin" {
		t.Fatalf("%q", env)
	}
	if i := slices.Index(args, "--settings"); i < 0 || args[i+1] != geminiSettings || args[len(args)-1] != "--print=P" {
		t.Fatalf("%q", args)
	}
	for _, a := range args {
		if strings.Contains(a, testGeminiSecret) {
			t.Fatalf("secret in argv: %q", args)
		}
	}
	if len(*asked) != 2 {
		t.Fatalf("%+v", *asked)
	}
	for _, r := range *asked {
		if r.TTL != time.Hour || (r.Name == "GEMINI_API_KEY" && r.URI != testGeminiRef) {
			t.Fatalf("%+v", r)
		}
	}
	// Another harness's table is never resolved for this one.
	*asked = nil
	if _, _, err = harnessLaunch(context.Background(), cfg, "claude", "test", nil, []string{"P"}); err != nil || len(*asked) != 0 {
		t.Fatalf("%v %+v", err, *asked)
	}
}

func TestHarnessLaunchResolutionErrors(t *testing.T) {
	cfg := writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiRef+"\"\n")
	fakeHarnessSecrets(t, "", errors.New("secrets endpoint unavailable"))
	if _, _, err := harnessLaunch(context.Background(), cfg, "agy", "test", nil, []string{"--print=P"}); err == nil || !strings.Contains(err.Error(), "harnesses.agy.env_refs") {
		t.Fatalf("%v", err)
	}
	old := resolveHarnessSecrets
	resolveHarnessSecrets = func(context.Context, []secrets.Ref, string) (map[string]string, error) {
		return map[string]string{"UNRELATED": testGeminiSecret}, nil
	}
	defer func() { resolveHarnessSecrets = old }()
	_, _, err := harnessLaunch(context.Background(), cfg, "agy", "test", nil, []string{"--print=P"})
	if err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY did not resolve") || strings.Contains(err.Error(), testGeminiSecret) {
		t.Fatalf("%v", err)
	}
}

// assertNoSecretOnDisk fails if value appears in any file under dir.
func assertNoSecretOnDisk(t *testing.T, dir, value string) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, e := os.ReadFile(p); e == nil && strings.Contains(string(b), value) {
			t.Errorf("secret value written to %s", p)
		}
		return nil
	})
}

func readArgsDump(t *testing.T, dump, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dump, name+".args"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

func checkAgyLaunch(t *testing.T, where string, args, env []string) {
	t.Helper()
	if envValue(env, "GEMINI_API_KEY") != testGeminiSecret {
		t.Fatalf("%s: GEMINI_API_KEY not injected", where)
	}
	i := slices.Index(args, "--settings")
	if i < 0 || i+2 >= len(args) || args[i+1] != geminiSettings || !strings.HasPrefix(args[i+2], "--print=") {
		t.Fatalf("%s: argv %q", where, args)
	}
}

// Both launch paths (a dispatch run's supervisor and an orchestrator round)
// inject the resolved key into agy's environment, pass the Gemini provider,
// and leave no trace of the value on disk.
func TestAgyLaunchPathsInjectSecretEnv(t *testing.T) {
	orchHome(t)
	allowChildProbes(t)
	asked := fakeHarnessSecrets(t, testGeminiSecret, nil)
	cfg := writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiRef+"\"\n")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	_ = os.Unsetenv("GEMINI_API_KEY")
	bin, dump := t.TempDir(), t.TempDir()
	installEnvDump(t, bin, "agy")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISPATCH_ENV_DUMP", dump)

	dir := filepath.Join(Root(), "agy-worker")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, LaunchEnv: []string{"DISPATCH_ENV_DUMP", "PATH"}, Order: Order{Name: "agy-worker", Repo: "app", Harness: "agy", Model: "gemini-4-carbon-high", Timeout: 1}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	_ = superviseRound("agy-worker")
	env := readEnvDump(t, dump, "agy")
	checkAgyLaunch(t, "dispatch run", readArgsDump(t, dump, "agy"), env)
	assertNoSecretOnDisk(t, dir, testGeminiSecret)

	s := applyOrch(t)
	s.Harness = "agy"
	_ = orchestratorSession(context.Background(), s, OrchestratorDir(s.Name), OrchestratorPacket{})
	env = readEnvDump(t, dump, "agy")
	checkAgyLaunch(t, "orchestrator round", readArgsDump(t, dump, "agy"), env)
	assertNoSecretOnDisk(t, OrchestratorDir(s.Name), testGeminiSecret)
	assertNoSecretOnDisk(t, OrchestratorConfigDir(), testGeminiSecret)
	if len(*asked) != 2 {
		t.Fatalf("expected one resolution per launch, got %+v", *asked)
	}
}

// A failed resolution stops the run before agy starts and records an error
// that names the table, never a value.
func TestAgyRunFailsWhenSecretsUnavailable(t *testing.T) {
	orchHome(t)
	allowChildProbes(t)
	fakeHarnessSecrets(t, "", errors.New("secrets endpoint unavailable"))
	cfg := writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiRef+"\"\n")
	bin, dump := t.TempDir(), t.TempDir()
	installEnvDump(t, bin, "agy")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISPATCH_ENV_DUMP", dump)
	dir := filepath.Join(Root(), "agy-nokey")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, LaunchEnv: []string{"DISPATCH_ENV_DUMP", "PATH"}, Order: Order{Name: "agy-nokey", Repo: "app", Harness: "agy", Timeout: 1}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	_ = superviseRound("agy-nokey")
	var r Result
	if err := readJSON(filepath.Join(dir, "result.json"), &r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Error, "harnesses.agy.env_refs") {
		t.Fatalf("result error %q", r.Error)
	}
	if _, err := os.Stat(filepath.Join(dump, "agy.env")); !os.IsNotExist(err) {
		t.Fatalf("agy started without its secret env: %v", err)
	}
}

func TestCopilotLaunchAndSessionCapture(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("fake Copilot uses a Unix shell")
	}
	orchHome(t)
	allowChildProbes(t)
	const token = "synthetic-copilot-test-token"
	asked := fakeHarnessSecrets(t, token, nil)
	cfg := writeDispatchConfig(t, "[harnesses.copilot.env_refs]\nCOPILOT_GITHUB_TOKEN = \"op://example/copilot/token\"\n")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	t.Setenv("COPILOT_HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
set -eu
[ "$COPILOT_GITHUB_TOKEN" = "synthetic-copilot-test-token" ]
if [ "$1" = "--session-id" ]; then
  id=$2
  shift 2
fi
[ "$1" = "--allow-all-tools" ]
[ "$2" = "--no-ask-user" ]
[ "$3" = "--model" ]
[ "$4" = "sample-model" ]
case "$5" in --prompt=*) ;; *) exit 8;; esac
[ "$#" -eq 5 ]
if [ -n "${id:-}" ] && [ ! -f ../skip-event ]; then
  dir="$COPILOT_HOME/session-state/$id"
  mkdir -p "$dir"
  printf '{"type":"session.start","data":{"sessionId":"%s"}}\n' "$id" > "$dir/events.jsonl"
else
  echo '{"session_id":"untrusted-output"}'
fi
`
	if err := os.WriteFile(filepath.Join(bin, "copilot"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, name := range []string{"with-event", "without-event"} {
		dir := filepath.Join(Root(), name)
		if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
			t.Fatal(err)
		}
		if name == "without-event" {
			if err := os.WriteFile(filepath.Join(dir, "skip-event"), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true,
			LaunchEnv: []string{"PATH", "COPILOT_HOME"},
			Order:     Order{Name: name, Repo: "app", Harness: "copilot", Model: "sample-model", Timeout: 1}}
		if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
			t.Fatal(err)
		}
		_ = superviseRound(name)
		var result Result
		if err := readJSON(filepath.Join(dir, "result.json"), &result); err != nil || result.ExitCode != 0 || result.Error != "" {
			t.Fatalf("worker result: %+v, %v", result, err)
		}
		if name == "with-event" {
			if result.Session == "" || !exists(filepath.Join(os.Getenv("COPILOT_HOME"), "session-state", result.Session, "events.jsonl")) {
				t.Fatalf("worker did not capture session: %+v", result)
			}
		} else if result.Session != "" {
			t.Fatalf("worker accepted a session ID from stdout: %q", result.Session)
		}
		assertNoSecretOnDisk(t, dir, token)
	}
	s := applyOrch(t)
	s.Harness, s.Model = "copilot", "sample-model"
	if err := orchestratorSession(ctx, s, OrchestratorDir(s.Name), OrchestratorPacket{}); err != nil {
		t.Fatal(err)
	}
	assertNoSecretOnDisk(t, OrchestratorDir(s.Name), token)
	if len(*asked) != 3 || (*asked)[0].Name != "COPILOT_GITHUB_TOKEN" {
		t.Fatalf("Copilot token resolution: %+v", *asked)
	}
}
