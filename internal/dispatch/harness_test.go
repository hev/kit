package dispatch

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const agyOrderBody = "---\nname: agy-run\nrepo: kit\nharness: agy\n%s---\nDo it.\n\n## Acceptance\n- done\n"

func agyOrder(extra string) string { return strings.Replace(agyOrderBody, "%s", extra, 1) }

func TestParseOrderAgyHarnessAndEffort(t *testing.T) {
	o, err := Parse(agyOrder("model: gemini-4-carbon\neffort: high\n"))
	if err != nil {
		t.Fatal(err)
	}
	if o.Harness != "agy" || o.Model != "gemini-4-carbon" || o.Effort != "high" {
		t.Fatalf("%+v", o)
	}
	for _, e := range Efforts {
		if o, err := Parse(agyOrder("effort: " + e + "\n")); err != nil || o.Effort != e {
			t.Fatalf("effort %s: %+v %v", e, o, err)
		}
	}
	// Effort applies to every harness, and is optional.
	if o, err := Parse(strings.Replace(agyOrder("effort: xhigh\n"), "harness: agy", "harness: claude", 1)); err != nil || o.Effort != "xhigh" {
		t.Fatalf("%+v %v", o, err)
	}
	if o, err := Parse(agyOrder("")); err != nil || o.Effort != "" {
		t.Fatalf("%+v %v", o, err)
	}
	for _, bad := range []string{"effort: extreme\n", "effort: HIGH\n", "effort: minimal\n"} {
		if _, err := Parse(agyOrder(bad)); err == nil || !strings.Contains(err.Error(), "invalid effort") {
			t.Fatalf("%q accepted: %v", bad, err)
		}
	}
	if _, err := Parse(strings.Replace(agyOrder(""), "harness: agy", "harness: gemini-cli", 1)); err == nil {
		t.Fatal("unknown harness accepted")
	}
}

func TestParseOrchestratorAgyHarnessAndEffort(t *testing.T) {
	spec := func(extra string) string {
		return "---\nkind: orchestrator\nname: agy-follow\ngoal: ship it\nharness: agy\nmodel: gemini-4-neon\n" + extra + "reply: foh-kit\nmanages:\n  runs: mine\n---\nBrief.\n"
	}
	s, err := ParseOrchestrator(spec("effort: medium\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Harness != "agy" || s.Model != "gemini-4-neon" || s.Effort != "medium" {
		t.Fatalf("%+v", s)
	}
	if s, err := ParseOrchestrator(spec("")); err != nil || s.Effort != "" {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := ParseOrchestrator(spec("effort: turbo\n")); err == nil || !strings.Contains(err.Error(), "invalid effort") {
		t.Fatalf("invalid effort accepted: %v", err)
	}
}

func TestCheckValidatorAgyAndEffort(t *testing.T) {
	o := Order{Harness: "claude", Model: "m1"}
	if err := CheckValidator(o, ValidationConfig{Harness: "agy", Model: "gemini-4-carbon", Effort: "max"}, false); err != nil {
		t.Fatal(err)
	}
	if err := CheckValidator(o, ValidationConfig{Harness: "agy", Model: "gemini-4-carbon", Effort: "ultra"}, false); err == nil {
		t.Fatal("invalid validation effort accepted")
	}
	if err := CheckValidator(o, ValidationConfig{Harness: "gemini-cli", Model: "x"}, false); err == nil {
		t.Fatal("unknown validation harness accepted")
	}
}

func TestAgyArgv(t *testing.T) {
	prompt := "--model trap: read ../order.md"
	cases := []struct {
		model, effort string
		want          []string
	}{
		{"", "", []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "--print=" + prompt}},
		{"gemini-4-carbon", "high", []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "--model", "gemini-4-carbon", "--effort", "high", "--print=" + prompt}},
		// A combined model name passes through; no --effort is invented.
		{"gemini-4-carbon-high", "", []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "--model", "gemini-4-carbon-high", "--print=" + prompt}},
		{"", "low", []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "--effort", "low", "--print=" + prompt}},
	}
	for _, c := range cases {
		got := harnessArgs("agy", c.model, c.effort, prompt)
		if !slices.Equal(got, c.want) {
			t.Fatalf("model %q effort %q:\n got %q\nwant %q", c.model, c.effort, got, c.want)
		}
		// --print never stands alone, so it can never swallow a later flag,
		// and it is the last argument.
		if slices.Contains(got, "--print") || !strings.HasPrefix(got[len(got)-1], "--print=") {
			t.Fatalf("%q", got)
		}
	}
	m := Meta{Order: Order{Harness: "agy", Model: "gemini-4-neon", Effort: "medium"}}
	if got, want := runArgs(m, "P"), []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "--model", "gemini-4-neon", "--effort", "medium", "--print=P"}; !slices.Equal(got, want) {
		t.Fatalf("run argv %q", got)
	}
	s := OrchestratorSpec{Harness: "agy", Model: "gemini-4-neon", Effort: "xhigh"}
	if got, want := orchestratorArgs(s, "R"), []string{"--dangerously-skip-permissions", "--output-format", "stream-json", "--model", "gemini-4-neon", "--effort", "xhigh", "--print=R"}; !slices.Equal(got, want) {
		t.Fatalf("orchestrator argv %q", got)
	}
	// The command line itself is agy followed by exactly these arguments.
	c := harnessCommand(nil, "agy", runArgs(m, "P"))
	if filepath.Base(c.Args[0]) != "agy" || !slices.Equal(c.Args[1:], runArgs(m, "P")) {
		t.Fatalf("%q", c.Args)
	}
}

func TestHarnessArgsUnchangedForClaudeAndCodex(t *testing.T) {
	if got, want := harnessArgs("claude", "opus", "", "P"), []string{"-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions", "--model", "opus", "P"}; !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
	if got, want := harnessArgs("codex", "", "", "P"), []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "P"}; !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
	// Follow-through orchestrators keep their default high effort, per harness.
	follow := Meta{Order: Order{Harness: "codex", Until: "all followed runs merged or abandoned"}}
	if got := runArgs(follow, "P"); !slices.Contains(got, `model_reasoning_effort="high"`) || got[len(got)-1] != "P" {
		t.Fatalf("%q", got)
	}
	follow.Harness = "claude"
	if got := runArgs(follow, "P"); !slices.Contains(got, "--effort") || got[slices.Index(got, "--effort")+1] != "high" {
		t.Fatalf("%q", got)
	}
	// An explicit effort wins over that default.
	follow.Effort = "max"
	if got := runArgs(follow, "P"); got[slices.Index(got, "--effort")+1] != "max" {
		t.Fatalf("%q", got)
	}
	if got := orchestratorArgs(OrchestratorSpec{Harness: "codex"}, "R"); got[len(got)-2] != "--skip-git-repo-check" || got[len(got)-1] != "R" {
		t.Fatalf("%q", got)
	}
}

func TestWireHooksSkipsAgy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "steer")
	in := harnessArgs("agy", "gemini-4-carbon", "high", "P")
	args, env := wireHooks("agy", dir, "dispatch:x", in, []string{"KEEP=1"})
	if !slices.Equal(args, in) || !slices.Equal(env, []string{"KEEP=1"}) {
		t.Fatalf("%q %q", args, env)
	}
	// No wired marker: hev steer treats the run as turn-boundary only.
	if _, err := os.Stat(filepath.Join(dir, wiredFile)); !os.IsNotExist(err) {
		t.Fatalf("wired marker written for agy: %v", err)
	}
}

func TestAgySkillRootAndSession(t *testing.T) {
	if got := harnessSkillRoot("agy"); got != filepath.Join(".agents", "skills") {
		t.Fatal(got)
	}
	log := filepath.Join(t.TempDir(), "log")
	line := `{"event":"result","result":{"conversation_id":"conv-123","status":"OK"}}`
	if err := os.WriteFile(log, []byte("not json\n"+line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := sessionFromLog(log); got != "conv-123" {
		t.Fatalf("%q", got)
	}
}

func TestDetectCallerAgy(t *testing.T) {
	for _, k := range []string{"CLAUDE_CODE_SESSION_ID", "CLAUDE_SESSION_ID", "CODEX_THREAD_ID", "CLAUDECODE"} {
		t.Setenv(k, "")
	}
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "conv-9")
	if c := DetectCaller(); c.Harness != "agy" || c.HarnessSession != "conv-9" {
		t.Fatalf("%+v", c)
	}
}
