package dispatch

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/weave"
)

// fakeHarness is built into claude, codex and gh. As a harness it records its
// standard input and arguments under $CONTEXT_CAPTURE and prints a session
// id; as gh it fails, keeping PR discovery off the network.
const fakeHarness = `package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if name == "gh" {
		os.Exit(1)
	}
	dir := os.Getenv("CONTEXT_CAPTURE")
	entries, _ := os.ReadDir(dir)
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".stdin") {
			n++
		}
	}
	base := filepath.Join(dir, fmt.Sprintf("%d-%s", n+1, name))
	in, _ := io.ReadAll(os.Stdin)
	_ = os.WriteFile(base+".args", []byte(strings.Join(os.Args[1:], "\x00")), 0600)
	_ = os.WriteFile(base+".stdin", in, 0600)
	fmt.Printf("{\"type\":\"system\",\"session_id\":\"%s-%d\"}\n", name, n+1)
}
`

type contextHarness struct {
	t         *testing.T
	workspace string
	capture   string
	launched  []string
}

func (h *contextHarness) git(dir string, args ...string) string {
	h.t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := c.CombinedOutput()
	if err != nil {
		h.t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// capture returns the stdin and arguments of the nth harness launch.
func (h *contextHarness) captured(n int, name string) (stdin []byte, args []string) {
	h.t.Helper()
	base := filepath.Join(h.capture, strings.Join([]string{itoa(n), name}, "-"))
	in, err := os.ReadFile(base + ".stdin")
	if err != nil {
		h.t.Fatalf("launch %d (%s) not captured: %v", n, name, err)
	}
	a, _ := os.ReadFile(base + ".args")
	return in, strings.Split(string(a), "\x00")
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func newContextHarness(t *testing.T) *contextHarness {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_DISPATCH_CONFIG", "")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}
	h := &contextHarness{t: t, workspace: t.TempDir(), capture: t.TempDir()}
	t.Setenv("CONTEXT_CAPTURE", h.capture)

	bin, src := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(fakeHarness), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module fakeharness\n\ngo 1.21\n"), 0600); err != nil {
		t.Fatal(err)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "claude"+exe), ".")
	build.Dir = src
	build.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake harness: %v: %s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(bin, "claude"+exe))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "gh"} {
		if err := os.WriteFile(filepath.Join(bin, name+exe), raw, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// origin holds the recipes and their sources at the base commit.
	origin := filepath.Join(t.TempDir(), "origin")
	files := map[string]string{
		"AGENTS.md":                   "# Fixture\nScope: all tasks.\nUse the fixture skill.\n",
		".claude/skills/fix/SKILL.md": "---\nname: fix\n---\nScope: fixing.\nFix carefully.\n",
		"recipes/ok.json":             `{"version":1,"task":"replaced by the order","sources":[{"path":"AGENTS.md","target":"AGENTS.md"},{"path":".claude/skills/fix/SKILL.md","target":".claude/skills/fix/SKILL.md"}]}` + "\n",
		"recipes/bad.json":            "{\"version\":1,\n",
		"recipes/missing-source.json": `{"version":1,"task":"t","sources":[{"path":"docs/nope.md","target":"nope.md"}]}` + "\n",
	}
	for name, body := range files {
		p := filepath.Join(origin, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h.git(filepath.Dir(origin), "init", "-q", "-b", "main", origin)
	h.git(origin, "-c", "core.autocrlf=false", "add", ".")
	h.git(origin, "commit", "-q", "-m", "fixture")
	h.git(h.workspace, "clone", "-q", "-c", "core.autocrlf=false", origin, "proj")

	before := runLauncher
	beforeAmend := amendLaunch
	t.Cleanup(func() { runLauncher, amendLaunch = before, beforeAmend })
	runLauncher = func(dir, name string) error { h.launched = append(h.launched, name); return nil }
	amendLaunch = runLauncher
	return h
}

func contextOrder(name, harness, recipe string) string {
	text := "---\nname: " + name + "\nrepo: proj\nharness: " + harness + "\n"
	if recipe != "" {
		text += "context: " + recipe + "\n"
	}
	return text + "---\nAdd the widget.\n\n## Acceptance\n- The widget exists.\n"
}

func (h *contextHarness) start(text string) error {
	h.t.Helper()
	o, err := Parse(text)
	if err != nil {
		h.t.Fatal(err)
	}
	// A sender-supplied bundle record, and the commit it would pin, is ignored.
	forged := &ContextRecord{Digest: "forged", RecipeCommit: "0000000000000000000000000000000000000000", Base: o.Base}
	_, err = Handle(Request{Action: "start", Name: o.Name, Order: text, Meta: Meta{Order: o, ContextDigest: "forged", ContextBundle: forged}, Workspace: h.workspace})
	return err
}

func runMeta(t *testing.T, name string) Meta {
	t.Helper()
	var m Meta
	if err := readJSON(filepath.Join(Root(), name, "meta.json"), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// runContextLifecycle drives recipe-driven runs through the real admission,
// worktree, bundle build and supervisor launch on this platform, with fake
// harness executables that record what they were given.
func runContextLifecycle(t *testing.T) {
	h := newContextHarness(t)
	launch := 0

	// Start: the bundle is built in the run record before the harness starts.
	if err := h.start(contextOrder("ctx-claude", "claude", "recipes/ok.json")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "ctx-claude")
	m := runMeta(t, "ctx-claude")
	c := m.ContextBundle
	if c == nil || m.ContextDigest == "" || c.Digest != m.ContextDigest || c.Bundle != "contexts/1" || c.Recipe != "recipes/ok.json" || c.Delivery != "stdin" || len(c.Sources) != 2 {
		t.Fatalf("context record %+v", c)
	}
	recipe, _ := os.ReadFile(filepath.Join(dir, "wt", "recipes", "ok.json"))
	if c.RecipeSHA256 != weave.Hash(recipe) || c.RecipeCommit != h.git(filepath.Join(dir, "wt"), "rev-parse", "origin/main") || c.Base != "main" {
		t.Fatalf("recipe identity %+v", c)
	}
	if strings.Join(c.HarnessAlsoLoads, ",") != ".claude/skills/fix/SKILL.md,AGENTS.md" || !strings.Contains(c.Note, "not everything the harness saw") {
		t.Fatalf("ambient record %+v", c)
	}
	bundle, err := weave.LoadBundle(filepath.Join(dir, "contexts", "1"))
	if err != nil || bundle.Digest != m.ContextDigest || bundle.Task != "Add the widget.\n\n## Acceptance\n- The widget exists." {
		t.Fatalf("bundle %+v %v", bundle.Task, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, "contexts", "1", "context.md"))
		if info == nil || info.Mode().Perm() != 0600 {
			t.Fatalf("bundle permissions %v", info)
		}
	}
	// The bundle never lands in the worktree, so it cannot be committed.
	if status := h.git(filepath.Join(dir, "wt"), "status", "--porcelain"); status != "" {
		t.Fatalf("worktree changed: %s", status)
	}
	if len(h.launched) != 1 {
		t.Fatalf("launched %v", h.launched)
	}
	if err := superviseRound("ctx-claude"); err != nil {
		t.Fatal(err)
	}
	launch++
	stdin, args := h.captured(launch, "claude")
	contextMD, _ := os.ReadFile(filepath.Join(dir, "contexts", "1", "context.md"))
	if !bytes.Equal(stdin, contextMD) || !bytes.Contains(stdin, []byte("# Assignment\n\nAdd the widget.")) || !bytes.Contains(stdin, []byte("Fix carefully.")) {
		t.Fatalf("claude stdin %q", stdin)
	}
	// The prompt argument is the unchanged worker prompt.
	if prompt := args[len(args)-1]; !strings.HasPrefix(prompt, Prompt) || strings.Contains(prompt, "Assignment") {
		t.Fatalf("prompt %q", prompt)
	}
	r := ctxResult(t, dir)
	first := m.ContextDigest
	if r.ContextDigest != first || r.Session != "claude-1" || r.Error != "" {
		t.Fatalf("result %+v", r)
	}
	if ContextAttribution(filepath.Join(dir, "wt"), "claude-1") != first {
		t.Fatal("session not attributed")
	}

	// ls and show carry the digest.
	list, err := Handle(Request{Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range list.Rows {
		found = found || (row.Name == "ctx-claude" && row.ContextDigest == first)
	}
	rowJSON, _ := json.Marshal(list)
	if !found || !strings.Contains(string(rowJSON), `"context_digest":"`+first+`"`) {
		t.Fatalf("ls rows %s", rowJSON)
	}
	show, err := Handle(Request{Action: "show", Name: "ctx-claude"})
	if err != nil || !strings.Contains(show.Files["meta.json"], `"context_digest": "`+first+`"`) || !strings.Contains(show.Files["result.json"], first) {
		t.Fatalf("show %v %s", err, show.Files["meta.json"])
	}

	// Rework: a new bundle for the new attempt; the previous one is kept.
	if _, err := Handle(Request{Action: "rework", Name: "ctx-claude", Note: "Also test the error case."}); err != nil {
		t.Fatal(err)
	}
	m = runMeta(t, "ctx-claude")
	second := m.ContextDigest
	if second == "" || second == first || m.ContextBundle.Bundle != "contexts/2" || m.ContextBundle.RecipeCommit != c.RecipeCommit {
		t.Fatalf("rework context %+v", m.ContextBundle)
	}
	if old, err := weave.LoadBundle(filepath.Join(dir, "contexts", "1")); err != nil || old.Digest != first {
		t.Fatalf("previous bundle lost: %v", err)
	}
	hist := history(dir, m)
	if len(hist.Attempts) != 2 || hist.Attempts[0].Meta.ContextDigest != first || hist.Attempts[1].Meta.ContextDigest != second || hist.Attempts[0].Result == nil || hist.Attempts[0].Result.ContextDigest != first {
		t.Fatalf("history %+v", hist.Attempts)
	}
	if err := superviseRound("ctx-claude"); err != nil {
		t.Fatal(err)
	}
	launch++
	if stdin, _ = h.captured(launch, "claude"); !bytes.Contains(stdin, []byte("## Rework\n\nAlso test the error case.")) {
		t.Fatalf("rework stdin %q", stdin)
	}
	if ContextAttribution(filepath.Join(dir, "wt"), "claude-1") != first || ContextAttribution(filepath.Join(dir, "wt"), "claude-2") != second {
		t.Fatal("sessions not joined to their attempts' contexts")
	}
	for attempt, want := range map[int]string{0: "2", 1: "1", 2: "2"} {
		got, err := ContextBundleDir("ctx-claude", attempt)
		if err != nil || got != filepath.Join(dir, "contexts", want) {
			t.Fatalf("bundle dir %d: %s %v", attempt, got, err)
		}
	}

	// Amend to codex on the ended run: a third attempt with its own bundle,
	// delivered to codex the same way.
	amended := contextOrder("ctx-claude", "codex", "recipes/ok.json")
	if _, err := Handle(Request{Action: "amend", Name: "ctx-claude", Order: amended}); err != nil {
		t.Fatal(err)
	}
	m = runMeta(t, "ctx-claude")
	if m.ContextBundle == nil || m.ContextBundle.Bundle != "contexts/3" || m.Harness != "codex" {
		t.Fatalf("amend context %+v", m.ContextBundle)
	}
	if err := superviseRound("ctx-claude"); err != nil {
		t.Fatal(err)
	}
	launch++
	stdin, args = h.captured(launch, "codex")
	contextMD, _ = os.ReadFile(filepath.Join(dir, "contexts", "3", "context.md"))
	if !bytes.Equal(stdin, contextMD) || args[0] != "exec" {
		t.Fatalf("codex stdin %q args %q", stdin, args)
	}

	// A fresh codex run with a recipe.
	if err := h.start(contextOrder("ctx-codex", "codex", "recipes/ok.json")); err != nil {
		t.Fatal(err)
	}
	if err := superviseRound("ctx-codex"); err != nil {
		t.Fatal(err)
	}
	launch++
	stdin, _ = h.captured(launch, "codex")
	contextMD, _ = os.ReadFile(filepath.Join(Root(), "ctx-codex", "contexts", "1", "context.md"))
	if !bytes.Equal(stdin, contextMD) || len(stdin) == 0 {
		t.Fatalf("codex start stdin %q", stdin)
	}

	// No recipe: exactly as before, no bundle and nothing on stdin.
	if err := h.start(contextOrder("ctx-none", "claude", "")); err != nil {
		t.Fatal(err)
	}
	if m = runMeta(t, "ctx-none"); m.ContextBundle != nil || m.ContextDigest != "" || m.Context != "" {
		t.Fatalf("no-recipe meta %+v", m)
	}
	if _, err := os.Stat(filepath.Join(Root(), "ctx-none", "contexts")); !os.IsNotExist(err) {
		t.Fatalf("no-recipe run has contexts: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(Root(), "ctx-none", "meta.json"))
	if strings.Contains(string(raw), "context") {
		t.Fatalf("no-recipe meta mentions context: %s", raw)
	}
	if err := superviseRound("ctx-none"); err != nil {
		t.Fatal(err)
	}
	launch++
	if stdin, _ = h.captured(launch, "claude"); len(stdin) != 0 {
		t.Fatalf("no-recipe stdin %q", stdin)
	}
	if r = ctxResult(t, filepath.Join(Root(), "ctx-none")); r.ContextDigest != "" || ContextAttribution(filepath.Join(Root(), "ctx-none", "wt"), r.Session) != "" {
		t.Fatalf("no-recipe result %+v", r)
	}

	// Failures name the problem and launch nothing.
	launchedBefore := len(h.launched)
	for name, c := range map[string]struct{ recipe, want string }{
		"ctx-missing":   {"recipes/nope.json", "context recipe recipes/nope.json not found at base commit"},
		"ctx-invalid":   {"recipes/bad.json", "invalid context recipe recipes/bad.json"},
		"ctx-no-source": {"recipes/missing-source.json", "context recipe recipes/missing-source.json: source docs/nope.md"},
	} {
		err := h.start(contextOrder(name, "claude", c.recipe))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: %v", name, err)
		}
		r := ctxResult(t, filepath.Join(Root(), name))
		if !r.AdmissionFailed || !strings.Contains(r.Error, c.want) || State(&r, false) != "failed" {
			t.Fatalf("%s result %+v", name, r)
		}
		if _, err := os.Stat(filepath.Join(Root(), name, "contexts", "1")); !os.IsNotExist(err) {
			t.Fatalf("%s left a partial bundle: %v", name, err)
		}
	}
	// A source removed from the worktree fails the rework before launch.
	if err := os.Remove(filepath.Join(dir, "wt", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(Request{Action: "rework", Name: "ctx-claude", Note: "Again."}); err == nil || !strings.Contains(err.Error(), "source AGENTS.md") {
		t.Fatalf("rework with missing source: %v", err)
	}
	if r = ctxResult(t, dir); r.ExitCode != 1 || !strings.Contains(r.Error, "source AGENTS.md") || State(&r, false) != "failed" {
		t.Fatalf("failed rework result %+v", r)
	}
	if m = runMeta(t, "ctx-claude"); m.ContextBundle != nil || m.ContextDigest != "" {
		t.Fatalf("failed rework recorded a bundle %+v", m.ContextBundle)
	}
	if len(h.launched) != launchedBefore {
		t.Fatalf("failure launched %v", h.launched[launchedBefore:])
	}
	// The supervisor refuses a recipe run with no verified bundle.
	if err := superviseRound("ctx-claude"); err != nil {
		t.Fatal(err)
	}
	if r = ctxResult(t, dir); !strings.Contains(r.Error, "has no bundle for this attempt") {
		t.Fatalf("supervisor guard %+v", r)
	}
	entries, _ := os.ReadDir(h.capture)
	if len(entries) != 2*launch {
		t.Fatalf("supervisor guard launched a harness: %d captures", len(entries))
	}
}

func TestContextLifecycle(t *testing.T) { runContextLifecycle(t) }

func TestOrderContextKey(t *testing.T) {
	o, err := Parse(contextOrder("ctx", "claude", "recipes/ok.json"))
	if err != nil || o.Context != "recipes/ok.json" {
		t.Fatalf("%+v %v", o, err)
	}
	o, err = Parse(contextOrder("ctx", "claude", ""))
	if err != nil || o.Context != "" {
		t.Fatalf("%+v %v", o, err)
	}
	for _, bad := range []string{"../outside.json", "/etc/recipe.json", "a/../b.json", "a\\b.json", "."} {
		if _, err := Parse(contextOrder("ctx", "claude", bad)); err == nil || !strings.Contains(err.Error(), "invalid context recipe path") {
			t.Errorf("accepted %q: %v", bad, err)
		}
	}
	if !restartOrder(Order{Context: "a.json"}, Order{Context: "b.json"}) {
		t.Fatal("changing the recipe must restart a live attempt")
	}
}

// An older target parses orders strictly and refuses the context: key before
// admission. The sender names the cause instead of the bare parse error.
func TestOlderTargetRefusesContextClearly(t *testing.T) {
	err := remoteError(Response{Error: "unknown key: context", Version: "0.9.0"})
	if err == nil || !strings.Contains(err.Error(), "target hev 0.9.0 does not support context recipes") {
		t.Fatalf("%v", err)
	}
	if err := remoteError(Response{Error: "unknown key: other"}); err.Error() != "unknown key: other" {
		t.Fatalf("%v", err)
	}
	// The mechanism: strict parsing rejects any key the version does not know.
	if _, err := Parse("---\nname: x\nrepo: r\nfuture_key: v\n---\n## Acceptance\n- x\n"); err == nil || err.Error() != "unknown key: future_key" {
		t.Fatalf("%v", err)
	}
}

func TestOrderIntent(t *testing.T) {
	if got := orderIntent("---\nname: x\n---\n\nDo it.\n\n## Acceptance\n- ok\n"); got != "Do it.\n\n## Acceptance\n- ok" {
		t.Fatalf("%q", got)
	}
}

func ctxResult(t *testing.T, dir string) Result {
	t.Helper()
	var r Result
	if err := readJSON(filepath.Join(dir, "result.json"), &r); err != nil {
		t.Fatal(err)
	}
	return r
}
