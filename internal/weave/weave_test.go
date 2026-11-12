package weave

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	p := t.TempDir() // macOS /var is an alias; source roots are explicitly canonical.
	p, e := filepath.EvalSymlinks(p)
	if e != nil {
		t.Fatal(e)
	}
	files := map[string]string{"AGENTS.md": "Always run tests.\nKeep user-facing messages short and provide concrete recovery steps.\n[missing](gone.md)\n<script>alert(1)</script>\n", "CLAUDE.md": "Never run tests.\nKeep user-facing messages short and provide concrete recovery steps.\n", ".claude/skills/test/SKILL.md": "Scope: testing\n[asset](assets/a.bin)\n", ".claude/skills/test/assets/a.bin": string([]byte{0, 255, 128, 1})}
	for n, b := range files {
		path := filepath.Join(p, n)
		if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, []byte(b), 0644); e != nil {
			t.Fatal(e)
		}
	}
	return p
}
func TestFlow(t *testing.T) {
	root := fixture(t)
	r, e := Discover(root)
	if e != nil {
		t.Fatal(e)
	}
	a := Assess(root, r)
	rules := map[string]bool{}
	for _, f := range a.Findings {
		rules[f.Rule] = true
		if f.Path == "" || f.Line < 1 {
			t.Fatal("finding lacks evidence")
		}
	}
	for _, rule := range []string{"possible-conflict", "unscoped", "missing-reference", "repeated-line"} {
		if !rules[rule] {
			t.Errorf("missing %s: %+v", rule, a)
		}
	}
	b, e := Build(root, r)
	if e != nil {
		t.Fatal(e)
	}
	again, _ := Build(root, r)
	if !bytes.Equal(encode(b), encode(again)) {
		t.Fatal("not deterministic")
	}
	out := filepath.Join(root, "bundle")
	if e = publish(out, bundleFiles(b)); e != nil {
		t.Fatal(e)
	}
	loaded, e := loadBundle(out)
	if e != nil || loaded.Digest != b.Digest {
		t.Fatal(e)
	}
	for _, f := range b.Files {
		raw, e := os.ReadFile(filepath.Join(out, "materialized", f.Target))
		if e != nil || !bytes.Equal(raw, f.Data) {
			t.Fatalf("lost bytes %s: %v", f.Target, e)
		}
	}
	r.Task = "Another assignment"
	r.Sources = r.Sources[:1]
	next, e := Build(root, r)
	if e != nil {
		t.Fatal(e)
	}
	diff := Diff(b, next)
	if len(diff.Changes) < 2 {
		t.Fatal("missing task/removal diff")
	}
	if e = publish(out, map[string][]byte{"context.md": []byte("overwrite")}); e == nil {
		t.Fatal("overwrote output")
	}
	for _, f := range b.Files {
		raw, _ := read(root, f.Path)
		if hash(raw) != f.SHA256 {
			t.Fatal("source changed")
		}
	}
	html := string(render(b, &diff))
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatal("raw injection")
	}
	if !strings.Contains(html, `\u003cscript\u003e`) {
		t.Fatal("missing escaped hostile content")
	}
}
func TestBadRecipesAndSources(t *testing.T) {
	root := fixture(t)
	for _, p := range []string{"../outside", "/etc/passwd", "a/../AGENTS.md", "a\\b", ".", ""} {
		_, e := Build(root, Recipe{1, "test", []Source{{p, "AGENTS.md"}}})
		if e == nil {
			t.Errorf("accepted path %q", p)
		}
	}
	if e := os.Symlink(filepath.Join(root, "AGENTS.md"), filepath.Join(root, "linked.md")); e != nil {
		t.Fatal(e)
	}
	for _, r := range []Recipe{{1, "test", []Source{{"linked.md", "AGENTS.md"}}}, {1, "test", []Source{{"missing", "x"}}}, {1, "test", []Source{{"AGENTS.md", "../out"}}}, {1, "test", []Source{{"AGENTS.md", "x"}, {"CLAUDE.md", "x"}}}, {1, "test", []Source{{"AGENTS.md", "x"}, {"CLAUDE.md", "x/y"}}}, {2, "test", []Source{{"AGENTS.md", "x"}}}} {
		if _, e := Build(root, r); e == nil {
			t.Errorf("accepted %+v", r)
		}
	}
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(root, "escape")); e != nil {
		t.Fatal(e)
	}
	if _, e := read(root, "escape/file"); e == nil {
		t.Fatal("ancestor escape")
	}
	r, _ := Discover(root)
	if len(r.Sources) != 4 {
		t.Fatal("unexpected discovery", r)
	}
}
func TestDuplicateProposal(t *testing.T) {
	root := fixture(t)
	b, _ := read(root, "AGENTS.md")
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), b, 0644)
	r, _ := Discover(root)
	a := Assess(root, r)
	if len(a.Improved.Sources) != len(r.Sources)-1 {
		t.Fatal("no concrete improvement")
	}
	if len(r.Sources) != 4 {
		t.Fatal("baseline mutated")
	}
}
func TestCLI(t *testing.T) {
	root := fixture(t)
	var w bytes.Buffer
	for _, cmd := range []string{"assess", "extract"} {
		out := filepath.Join(root, cmd)
		if e := Run([]string{cmd, "--root", root, "--out", out}, &w); e != nil {
			t.Fatal(e)
		}
	}
	var r Recipe
	if e := load(filepath.Join(root, "extract", "recipe.json"), &r); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(root, "recipe.json")
	os.WriteFile(p, encode(r), 0644)
	if e := Run([]string{"build", "--root", filepath.Join(root, "extract", "sources"), "--recipe", p, "--out", filepath.Join(root, "built")}, &w); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`{"version":1,"unknown":1}`, `{} {}`, `broken`} {
		os.WriteFile(p, []byte(raw), 0644)
		if e := Run([]string{"build", "--recipe", p, "--out", filepath.Join(root, "bad")}, &w); e == nil {
			t.Fatal("malformed accepted")
		}
	}
	if e := Run([]string{"review", "--bundle", filepath.Join(root, "built"), "--addr", "0.0.0.0:0"}, &w); e == nil {
		t.Fatal("public bind")
	}
	bundlePath := filepath.Join(root, "built", "bundle.json")
	var b Bundle
	load(bundlePath, &b)
	b.Task = "tampered"
	os.WriteFile(bundlePath, encode(b), 0644)
	if _, e := loadBundle(filepath.Dir(bundlePath)); e == nil {
		t.Fatal("tampering undetected")
	}
}
func TestManifestOrder(t *testing.T) {
	root := fixture(t)
	r, _ := Discover(root)
	a, _ := Build(root, r)
	r.Sources[0], r.Sources[1] = r.Sources[1], r.Sources[0]
	b, _ := Build(root, r)
	if len(Diff(a, b).Changes) == 0 {
		t.Fatal("reorder missing from diff")
	}
	if a.Digest == b.Digest {
		t.Fatal("order not hashed")
	}
	var decoded Bundle
	if e := json.Unmarshal(encode(a), &decoded); e != nil {
		t.Fatal(e)
	}
}

func TestRelocatedSources(t *testing.T) {
	a, b := fixture(t), fixture(t)
	r, _ := Discover(a)
	x, e := Build(a, r)
	if e != nil {
		t.Fatal(e)
	}
	y, e := Build(b, r)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(encode(x), encode(y)) {
		t.Fatal("bundle embeds environment-dependent paths")
	}
}
