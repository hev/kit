package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
