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
	// A second harness's table, for fallback chains that cross harnesses.
	testClaudeEnv    = "HARNESS_TEST_TOKEN"
	testClaudeRef    = "op://example/claude-token/credential"
	testClaudeSecret = "claude-secret-value-2b8c44"
)

// unsetEnv removes name for the rest of the test and restores it after.
func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	_ = os.Unsetenv(name)
}

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
	return fakeHarnessSecretValues(t, func(secrets.Ref) string { return value }, err)
}

// fakeHarnessSecretValues is fakeHarnessSecrets with a value per ref.
func fakeHarnessSecretValues(t *testing.T, value func(secrets.Ref) string, err error) *[]secrets.Ref {
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
			out[r.Name] = value(r)
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
plan = "ultra"

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
		if agy.ModelProvider != "gemini" || agy.Plan != "ultra" || agy.EnvRefs["GEMINI_API_KEY"] != testGeminiRef {
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
	gotEnv, gotArgs, _, err := harnessLaunch(context.Background(), cfg, "agy", "test", env, args)
	if err != nil || !slices.Equal(gotEnv, env) || !slices.Equal(gotArgs, args) || len(*asked) != 0 {
		t.Fatalf("%q %q %v asked=%v", gotEnv, gotArgs, err, *asked)
	}
	// A missing config file is the same as no table.
	if _, _, _, err = harnessLaunch(context.Background(), filepath.Join(t.TempDir(), "none.toml"), "agy", "test", env, args); err != nil {
		t.Fatal(err)
	}
}

func TestHarnessConfigRejectsInvalid(t *testing.T) {
	for body, want := range map[string]string{
		"[harnesses.gpt]\nmodel_provider = \"gemini\"\n":                   "harnesses.gpt",
		"[harnesses.claude]\nmodel_provider = \"gemini\"\n":                "only agy",
		"[harnesses.agy]\nmodel_provider = \"bad provider\"\n":             "invalid provider",
		"[harnesses.agy]\nplan = \"bad/plan\"\n":                           "invalid plan",
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
	env, args, values, err := harnessLaunch(context.Background(), cfg, "agy", "test", []string{"PATH=/bin", "GEMINI_API_KEY=inherited"}, harnessArgs("agy", "gemini-4-carbon-high", "", "P"))
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
	if len(*asked) != 2 || len(values) != 2 || values[0] != testGeminiSecret {
		t.Fatalf("%+v %d", *asked, len(values))
	}
	for _, r := range *asked {
		if r.TTL != time.Hour || (r.Name == "GEMINI_API_KEY" && r.URI != testGeminiRef) {
			t.Fatalf("%+v", r)
		}
	}
	// Another harness's table is never resolved for this one.
	*asked = nil
	if _, _, _, err = harnessLaunch(context.Background(), cfg, "claude", "test", nil, []string{"P"}); err != nil || len(*asked) != 0 {
		t.Fatalf("%v %+v", err, *asked)
	}
}

func TestHarnessLaunchResolutionErrors(t *testing.T) {
	cfg := writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiRef+"\"\n")
	fakeHarnessSecrets(t, "", errors.New("secrets endpoint unavailable"))
	// The error names the table and the environment names, never a ref.
	if _, _, _, err := harnessLaunch(context.Background(), cfg, "agy", "test", nil, []string{"--print=P"}); err == nil || !strings.Contains(err.Error(), "harnesses.agy.env_refs (GEMINI_API_KEY)") || strings.Contains(err.Error(), testGeminiRef) {
		t.Fatalf("%v", err)
	}
	old := resolveHarnessSecrets
	resolveHarnessSecrets = func(context.Context, []secrets.Ref, string) (map[string]string, error) {
		return map[string]string{"UNRELATED": testGeminiSecret}, nil
	}
	defer func() { resolveHarnessSecrets = old }()
	_, _, _, err := harnessLaunch(context.Background(), cfg, "agy", "test", nil, []string{"--print=P"})
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
	if hasEnv(env, testClaudeEnv) {
		t.Fatalf("%s: agy received another harness's env_refs", where)
	}
	i := slices.Index(args, "--settings")
	if i < 0 || i+2 >= len(args) || args[i+1] != geminiSettings || !strings.HasPrefix(args[i+2], "--print=") {
		t.Fatalf("%s: argv %q", where, args)
	}
}

// Both launch paths (a dispatch run's supervisor and an orchestrator round)
// inject the resolved key into agy's environment, pass the Gemini provider,
// redact the value from agy's output, and leave no trace of it on disk.
func TestAgyLaunchPathsInjectSecretEnv(t *testing.T) {
	orchHome(t)
	allowChildProbes(t)
	asked := fakeHarnessSecrets(t, testGeminiSecret, nil)
	cfg := writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiRef+"\"\n")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	unsetEnv(t, "GEMINI_API_KEY")
	bin, dump := t.TempDir(), t.TempDir()
	installEnvDump(t, bin, "agy")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISPATCH_ENV_DUMP", dump)
	// The fake agy prints the key it received, split across stdout and stderr.
	t.Setenv("DISPATCH_ECHO_ENV", "GEMINI_API_KEY")

	dir := filepath.Join(Root(), "agy-worker")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, LaunchEnv: []string{"DISPATCH_ENV_DUMP", "DISPATCH_ECHO_ENV", "PATH"}, Order: Order{Name: "agy-worker", Repo: "app", Harness: "agy", Model: "gemini-4-carbon-high", Timeout: 1}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	_ = superviseRound("agy-worker")
	env := readEnvDump(t, dump, "agy")
	checkAgyLaunch(t, "dispatch run", readArgsDump(t, dump, "agy"), env)
	checkRedacted(t, filepath.Join(dir, "log"))
	assertNoSecretOnDisk(t, dir, testGeminiSecret)

	// A fresh dump directory, so the round cannot pass on the worker's files.
	dump = t.TempDir()
	t.Setenv("DISPATCH_ENV_DUMP", dump)
	s := applyOrch(t)
	s.Harness = "agy"
	if err := orchestratorSession(context.Background(), s, OrchestratorDir(s.Name), OrchestratorPacket{}); err != nil {
		t.Fatalf("orchestrator round: %v", err)
	}
	env = readEnvDump(t, dump, "agy")
	checkAgyLaunch(t, "orchestrator round", readArgsDump(t, dump, "agy"), env)
	checkRedacted(t, filepath.Join(OrchestratorDir(s.Name), "trace"))
	assertNoSecretOnDisk(t, OrchestratorDir(s.Name), testGeminiSecret)
	assertNoSecretOnDisk(t, OrchestratorConfigDir(), testGeminiSecret)
	if len(*asked) != 2 {
		t.Fatalf("expected one resolution per launch, got %+v", *asked)
	}
}

// checkRedacted requires the fake agy's diagnostic in path with the key
// replaced by the redaction marker.
func checkRedacted(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "diagnostic: key="+redactedMarker+" end") {
		t.Fatalf("%s: diagnostic not redacted: %q", path, b)
	}
}

func TestRedactingWriterSpansWrites(t *testing.T) {
	const short = "abc"
	in := "x " + testGeminiSecret + " y " + short + testGeminiSecret + " z" + testGeminiSecret[:4]
	want := "x " + redactedMarker + " y " + redactedMarker + redactedMarker + " z" + testGeminiSecret[:4]
	// Every split point, including ones inside a value.
	for i := 0; i <= len(in); i++ {
		for j := i; j <= len(in); j += 7 {
			var out strings.Builder
			r := newRedactingWriter(&out, []string{short, testGeminiSecret, "", short})
			for _, part := range []string{in[:i], in[i:j], in[j:]} {
				if n, err := r.Write([]byte(part)); err != nil || n != len(part) {
					t.Fatalf("write %d %v", n, err)
				}
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			if out.String() != want {
				t.Fatalf("split %d,%d: %q", i, j, out.String())
			}
		}
	}
	if newRedactingWriter(&strings.Builder{}, []string{""}) != nil {
		t.Fatal("redactor with no values")
	}
}

// redactAll writes parts through a fresh redactor over values and returns
// what reached the underlying writer.
func redactAll(t *testing.T, values []string, max int, parts ...string) string {
	t.Helper()
	var out strings.Builder
	r := newRedactingWriter(&out, values)
	if max > 0 {
		r.max = max
	}
	for _, part := range parts {
		if n, err := r.Write([]byte(part)); err != nil || n != len(part) {
			t.Fatalf("write %d %v", n, err)
		}
		if limit := r.max + r.longest; len(r.pending) > limit {
			t.Fatalf("held %d bytes, cap %d", len(r.pending), limit)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// Overlapping values are redacted by the full extent of their merged
// matches, whatever the write boundaries: a shorter value completing
// first never releases the rest of a longer one, and text that is not a
// complete value is kept byte for byte.
func TestRedactingWriterOverlappingValues(t *testing.T) {
	const m = redactedMarker
	for _, tc := range []struct {
		name   string
		values []string
		in     string
		want   string
	}{
		{"prefix", []string{"demo_PUBLIC", "demo_PUBLIC-private-suffix"},
			"a demo_PUBLIC-private-suffix b demo_PUBLIC c demo_PUBLIC-priv d demo_PUBLIC-private-suffi",
			"a " + m + " b " + m + " c " + m + "-priv d " + m + "-private-suffi"},
		{"suffix", []string{"private-suffix", "demo_PUBLIC-private-suffix"},
			"<demo_PUBLIC-private-suffix> <private-suffix> <demo_PUBLIC-private>",
			"<" + m + "> <" + m + "> <demo_PUBLIC-private>"},
		{"overlap", []string{"abc", "bcdef"},
			"x abcdef y bcd z abc w bcdefabc",
			"x " + m + " y bcd z " + m + " w " + m + m},
		{"chain", []string{"aa"}, "baaaab a b", "b" + m + "b a b"},
		{"inside", []string{"KEY", "xKEYx"}, "1 xKEYx 2 xKEY 3", "1 " + m + " 2 x" + m + " 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactAll(t, tc.values, 0, tc.in); got != tc.want {
				t.Fatalf("one write: %q, want %q", got, tc.want)
			}
			for i := 0; i <= len(tc.in); i++ {
				for j := i; j <= len(tc.in); j++ {
					if got := redactAll(t, tc.values, 0, tc.in[:i], tc.in[i:j], tc.in[j:]); got != tc.want {
						t.Fatalf("split %d,%d: %q, want %q", i, j, got, tc.want)
					}
				}
			}
			bytewise := strings.Split(tc.in, "")
			if got := redactAll(t, tc.values, 0, bytewise...); got != tc.want {
				t.Fatalf("byte at a time: %q, want %q", got, tc.want)
			}
		})
	}
}

// A run of overlapping matches longer than the hold cap is written out
// in pieces rather than held without bound, and still releases no byte of
// a value.
func TestRedactingWriterCapsHeldOutput(t *testing.T) {
	values := []string{"aa", "abcab"}
	in := "<" + strings.Repeat("a", 200) + "bcab" + strings.Repeat("a", 50) + ">"
	for _, chunk := range []int{1, 3, 7, 64} {
		var parts []string
		for i := 0; i < len(in); i += chunk {
			parts = append(parts, in[i:min(i+chunk, len(in))])
		}
		got := redactAll(t, values, 16, parts...)
		rest := strings.ReplaceAll(got, redactedMarker, "")
		if rest != "<>" || !strings.HasPrefix(got, "<"+redactedMarker) {
			t.Fatalf("chunk %d: %q", chunk, got)
		}
	}
}

// Windows matches environment names case-insensitively, so an inherited
// Gemini_Api_Key reaches agy and must select the Gemini provider there.
func TestEnvValueFollowsPlatformNameMatching(t *testing.T) {
	old := envFoldNames
	t.Cleanup(func() { envFoldNames = old })
	cases := []struct {
		env        []string
		fold, want string
	}{
		{[]string{"Gemini_Api_Key=k"}, "k", ""},
		{[]string{"GEMINI_API_KEY=k", "gemini_api_key="}, "", "k"},
		{[]string{"gemini_api_key=", "GEMINI_API_KEY=k"}, "k", "k"},
		{[]string{"GEMINI_API_KEY=a", "Gemini_Api_Key=b"}, "b", "a"},
	}
	for _, c := range cases {
		envFoldNames = true
		if got := envValue(c.env, "GEMINI_API_KEY"); got != c.fold {
			t.Fatalf("windows %q: %q", c.env, got)
		}
		envFoldNames = false
		if got := envValue(c.env, "GEMINI_API_KEY"); got != c.want {
			t.Fatalf("unix %q: %q", c.env, got)
		}
	}
	envFoldNames = true
	if p := harnessModelProvider("agy", HarnessConfig{}, []string{"Gemini_Api_Key=k"}); p != "gemini" {
		t.Fatalf("windows provider %q", p)
	}
	envFoldNames = false
	if p := harnessModelProvider("agy", HarnessConfig{}, []string{"Gemini_Api_Key=k"}); p != "" {
		t.Fatalf("unix provider %q", p)
	}
}

// env_refs cannot restore a name a source plugin declared plugin-only, on
// either launch path, and nothing is resolved when they collide.
func TestHarnessEnvRefsRejectPluginOnlyNames(t *testing.T) {
	orchHome(t)
	allowChildProbes(t)
	asked := fakeHarnessSecrets(t, testGeminiSecret, nil)
	text := strings.Replace(orchestratorExample, "source: none", "source:\n  plugin: env-source\n  env: [gemini_api_key]", 1)
	s, err := ApplyOrchestrator(text, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := writeDispatchConfig(t, "[harnesses.agy.env_refs]\nGEMINI_API_KEY = \""+testGeminiRef+"\"\n")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	bin, dump := t.TempDir(), t.TempDir()
	installEnvDump(t, bin, "agy")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISPATCH_ENV_DUMP", dump)
	const want = "harnesses.agy.env_refs.GEMINI_API_KEY: reserved for a source plugin"

	dir := filepath.Join(Root(), "agy-plugin")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, LaunchEnv: []string{"DISPATCH_ENV_DUMP", "PATH"}, Order: Order{Name: "agy-plugin", Repo: "app", Harness: "agy", Timeout: 1}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	_ = superviseRound("agy-plugin")
	var r Result
	if err := readJSON(filepath.Join(dir, "result.json"), &r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Error, want) {
		t.Fatalf("worker error %q", r.Error)
	}

	s.Harness = "agy"
	if err := orchestratorSession(context.Background(), s, OrchestratorDir(s.Name), OrchestratorPacket{}); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("round error %v", err)
	}
	if len(*asked) != 0 {
		t.Fatalf("resolved despite the collision: %+v", *asked)
	}
	if _, err := os.Stat(filepath.Join(dump, "agy.env")); !os.IsNotExist(err) {
		t.Fatalf("agy started with a plugin-only name: %v", err)
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
