package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/trace"
)

func runContextCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := newContextCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func TestContextCommandSurface(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	os.MkdirAll(filepath.Join(project, ".claude", "skills", "x"), 0700)
	os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("Scope: all.\nRun tests.\n"), 0600)
	os.WriteFile(filepath.Join(project, ".claude", "skills", "x", "SKILL.md"), []byte("Scope: x.\n"), 0600)
	for _, args := range [][]string{
		{"assess", "--root", project, "--out", filepath.Join(root, "assessment")},
		{"extract", "--root", project, "--out", filepath.Join(root, "extracted")},
		{"build", "--root", project, "--recipe", filepath.Join(root, "extracted", "recipe.json"), "--out", filepath.Join(root, "a")},
		{"build", "--root", filepath.Join(root, "extracted", "sources"), "--recipe", filepath.Join(root, "extracted", "improved.recipe.json"), "--out", filepath.Join(root, "b")},
		{"diff", "--before", filepath.Join(root, "a"), "--after", filepath.Join(root, "b"), "--out", filepath.Join(root, "d")},
	} {
		if out, err := runContextCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	// Same refusals as Weave: existing output, public review address.
	if _, err := runContextCLI(t, "build", "--root", project, "--recipe", filepath.Join(root, "extracted", "recipe.json"), "--out", filepath.Join(root, "a")); err == nil || !strings.Contains(err.Error(), "output already exists") {
		t.Fatalf("overwrite: %v", err)
	}
	if _, err := runContextCLI(t, "review", "--bundle", filepath.Join(root, "a"), "--addr", "0.0.0.0:0"); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("public bind: %v", err)
	}
	if out, err := runContextCLI(t, "build", "--help"); err != nil || !strings.Contains(out, "-recipe") {
		t.Fatalf("help: %v %s", err, out)
	}
	if out, _ := runContextCLI(t); !strings.Contains(out, "hev context review  --run NAME") {
		t.Fatalf("usage: %s", out)
	}
}

func TestContextReviewRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(dispatch.Root(), "ctx-run")
	os.MkdirAll(filepath.Join(dir, "contexts", "1"), 0700)
	m := dispatch.Meta{Order: dispatch.Order{Name: "ctx-run", Context: "r.json"}, ContextDigest: "d", ContextBundle: &dispatch.ContextRecord{Digest: "d", Bundle: "contexts/1"}}
	dispatch.WriteJSON(filepath.Join(dir, "meta.json"), m)
	got, err := reviewRunArgs([]string{"--run", "ctx-run", "--addr=127.0.0.1:0"})
	if err != nil || !reflect.DeepEqual(got, []string{"--addr=127.0.0.1:0", "--bundle", filepath.Join(dir, "contexts", "1")}) {
		t.Fatalf("%v %v", got, err)
	}
	if got, err = reviewRunArgs([]string{"--run=ctx-run", "--attempt", "1"}); err != nil || got[1] != filepath.Join(dir, "contexts", "1") {
		t.Fatalf("attempt: %v %v", got, err)
	}
	for _, bad := range [][]string{{"--run", "ctx-run", "--bundle", "x"}, {"--attempt", "1"}, {"--run", "missing"}, {"--run", "ctx-run", "--attempt", "0"}, {"--run"}} {
		if _, err := reviewRunArgs(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if got, _ := reviewRunArgs([]string{"--bundle", "x"}); !reflect.DeepEqual(got, []string{"--bundle", "x"}) {
		t.Fatalf("passthrough %v", got)
	}
}

func TestShowContext(t *testing.T) {
	var out bytes.Buffer
	// Records written before context recipes print nothing and do not fail.
	showContext(&out, `{"name":"old","repo":"r","branch":"d/old","start":"2026-01-01T00:00:00Z"}`)
	showContext(&out, ``)
	if out.Len() != 0 {
		t.Fatalf("%q", out.String())
	}
	m := dispatch.Meta{Order: dispatch.Order{Name: "ctx-run", Context: "recipes/ok.json"}, ContextDigest: strings.Repeat("e", 64), ContextBundle: &dispatch.ContextRecord{Digest: strings.Repeat("e", 64), Recipe: "recipes/ok.json", RecipeSHA256: "abc", RecipeCommit: "0123456789abcdef", Base: "main", Bundle: "contexts/2", Delivery: "stdin", HarnessAlsoLoads: []string{"AGENTS.md"}}}
	raw, _ := json.Marshal(m)
	showContext(&out, string(raw))
	for _, want := range []string{"Context: digest " + strings.Repeat("e", 64), "recipe recipes/ok.json sha256 abc at 0123456789ab (origin/main)", "bundle contexts/2", "hev context review --run ctx-run", "harness may also load on its own: AGENTS.md"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
	if got := attemptContext(m); got != ", context eeeeeeeeeeee" {
		t.Fatalf("%q", got)
	}
}

func TestContextFilters(t *testing.T) {
	d := strings.Repeat("f", 64)
	if got := sessionScopeFilter(nil, "", d); !reflect.DeepEqual(got, []any{"context_digest", "Eq", d}) {
		t.Fatalf("%v", got)
	}
	window := []any{"start", "Gte", 1}
	want := []any{"And", []any{[]any{"And", []any{window, []any{"order", "Eq", "o"}}}, []any{"context_digest", "Eq", d}}}
	if got := sessionScopeFilter(window, "o", d); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if got := sessionScopeFilter(nil, "", ""); got != nil {
		t.Fatalf("%v", got)
	}
	rows := []trace.SessionRow{{SessionID: "b"}, {SessionID: "a"}, {SessionID: "b"}}
	if got := contextChunkFilter(nil, rows); !reflect.DeepEqual(got, []any{"session_id", "In", []string{"a", "b"}}) {
		t.Fatalf("%v", got)
	}
	if !orderUsesTimeWindow("", false, false, 0) || orderUsesTimeWindow("o"+"", false, false, 0) || orderUsesTimeWindow(""+d, false, false, 0) {
		t.Fatal("--context should search all ages like --order")
	}
}

func TestContextPlan(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	os.MkdirAll(filepath.Join(project, ".claude", "skills", "x"), 0700)
	agents := "# Project\nIntro.\n## Build\nmake\n## Release\nsteps\n"
	os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte(agents), 0600)
	os.WriteFile(filepath.Join(project, ".claude", "skills", "x", "SKILL.md"), []byte("Scope: x.\n"), 0600)
	order := filepath.Join(root, "order.md")
	os.WriteFile(order, []byte("---\nname: fix-build\nrepo: project\n---\nFix the build.\n\n## Acceptance\n- make passes\n"), 0600)

	out := filepath.Join(root, "plan")
	got, err := runContextCLI(t, "plan", order, "--root", project, "--out", out)
	if err != nil {
		t.Fatalf("%v: %s", err, got)
	}
	for _, want := range []string{"2 sources, ", "instruction", "AGENTS.md", "## Release", "skill", "plan → " + out} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary lacks %q:\n%s", want, got)
		}
	}
	// The starting recipe is mechanical: every source, whole, the order body
	// as its task. It builds as is.
	raw, _ := os.ReadFile(filepath.Join(out, "recipe.json"))
	var r struct {
		Version int
		Task    string
		Sources []map[string]any
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.Task != "Fix the build.\n\n## Acceptance\n- make passes" || len(r.Sources) != 2 {
		t.Fatalf("recipe %s %v", raw, err)
	}
	for _, s := range r.Sources {
		if len(s) != 2 {
			t.Fatalf("starting recipe narrows: %v", s)
		}
	}
	var p struct {
		Files []struct {
			Path     string
			Bytes    int
			Sections []struct {
				Heading string
				Bytes   int
			}
			Findings []any
		}
	}
	raw, _ = os.ReadFile(filepath.Join(out, "plan.json"))
	if err := json.Unmarshal(raw, &p); err != nil || len(p.Files) != 2 || p.Files[1].Path != "AGENTS.md" || p.Files[1].Bytes != len(agents) || len(p.Files[1].Sections) != 3 || p.Files[1].Sections[2].Heading != "## Release" {
		t.Fatalf("plan %s %v", raw, err)
	}
	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !bytes.Contains(page, []byte("## Release")) || !bytes.Contains(page, []byte("Starting recipe")) || bytes.Contains(page, []byte("<script")) {
		t.Fatalf("review page %s", page)
	}
	if _, err := runContextCLI(t, "build", "--root", project, "--recipe", filepath.Join(out, "recipe.json"), "--out", filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	// A narrowed edit of it builds and reports the sizes.
	narrowed := strings.Replace(string(mustRead(t, filepath.Join(out, "recipe.json"))), `"target": "AGENTS.md"`, `"target": "AGENTS.md", "drop": ["## Release"]`, 1)
	os.WriteFile(filepath.Join(root, "narrow.json"), []byte(narrowed), 0600)
	got, err = runContextCLI(t, "build", "--root", project, "--recipe", filepath.Join(root, "narrow.json"), "--out", filepath.Join(root, "c"))
	if err != nil || !strings.Contains(got, "AGENTS.md: "+strconv.Itoa(len("# Project\nIntro.\n## Build\nmake\n"))+" of "+strconv.Itoa(len(agents))+" bytes (kept 2 sections, dropped 1)") {
		t.Fatalf("%v %s", err, got)
	}

	// No instruction files: nothing to narrow, nothing written.
	empty := filepath.Join(root, "empty")
	os.MkdirAll(empty, 0700)
	got, err = runContextCLI(t, "plan", order, "--root", empty, "--out", filepath.Join(root, "none"))
	if err != nil || !strings.Contains(got, "nothing to narrow") {
		t.Fatalf("%v %s", err, got)
	}
	if _, err := os.Stat(filepath.Join(root, "none")); !os.IsNotExist(err) {
		t.Fatal("plan wrote output for a repository without instruction files")
	}
	if _, err := runContextCLI(t, "plan", order, "--root", project); err == nil {
		t.Fatal("plan without --out")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// --context-file carries a checked recipe in the order; a file that is not a
// recipe (such as a prebuilt bundle) is refused before anything is sent.
func TestCarryContextFile(t *testing.T) {
	dir := t.TempDir()
	order := []byte("---\nname: x\nrepo: r\n---\nDo it.\n\n## Acceptance\n- ok\n")
	recipe := filepath.Join(dir, "r.json")
	os.WriteFile(recipe, []byte(`{"version":1,"task":"t","sources":[{"path":"AGENTS.md","target":"AGENTS.md","keep":["## Build"]}]}`), 0600)
	got, err := carryContextFile(order, recipe)
	if err != nil {
		t.Fatal(err)
	}
	o, err := dispatch.Parse(string(got))
	if err != nil || o.ContextRecipe == "" || dispatch.OrderIntent(string(got)) != "Do it.\n\n## Acceptance\n- ok" {
		t.Fatalf("%v %s", err, got)
	}
	if same, _ := carryContextFile(order, ""); !bytes.Equal(same, order) {
		t.Fatal("no --context-file changed the order")
	}
	bundle := filepath.Join(dir, "bundle.json")
	os.WriteFile(bundle, []byte(`{"version":1,"task":"t","digest":"abc","files":[],"context":"x","assessment":{}}`), 0600)
	if _, err := carryContextFile(order, bundle); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("bundle accepted: %v", err)
	}
}
