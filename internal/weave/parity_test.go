package weave

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Weave's synthetic example project (examples/project at github.com/hev/weave
// main, commit cb360fc), held here as data so its deliberately conflicting
// AGENTS.md/CLAUDE.md never sit in kit's tree where a harness would load them.
var exampleProject = map[string]string{
	"AGENTS.md":                                   "# Lantern sample project\nScope: all project tasks.\nAlways run tests.\nKeep user-facing messages short and provide concrete recovery steps.\nSee [architecture](docs/missing.md) before changing storage.\n<script>window.weaveHostile = true; alert('hostile')</script>\n",
	"legacy/AGENTS.md":                            "# Lantern sample project\nScope: all project tasks.\nAlways run tests.\nKeep user-facing messages short and provide concrete recovery steps.\nSee [architecture](docs/missing.md) before changing storage.\n<script>window.weaveHostile = true; alert('hostile')</script>\n",
	"CLAUDE.md":                                   "# Legacy local guidance\nNever run tests.\nKeep user-facing messages short and provide concrete recovery steps.\n",
	".claude/agents/reviewer.md":                  "---\nname: reviewer\ndescription: Review a synthetic Lantern change.\n---\nScope: review assignments only.\nLook for missing negative cases and explain the user impact.\n",
	".claude/skills/testing/SKILL.md":             "---\nname: testing\ndescription: Test a synthetic Lantern feature.\n---\nScope: testing assignments only.\nUse [the checklist](assets/checklist.txt) and run the focused tests.\n",
	".claude/skills/testing/assets/checklist.txt": "Check the happy path, invalid inputs, and preservation of existing files.\n",
}

// Weave's examples/review.recipe.json and examples/test.recipe.json, verbatim.
const reviewRecipe = `{
  "version": 1,
  "task": "Review the Lantern storage change; report risks with source evidence.",
  "sources": [
    {"path": "AGENTS.md", "target": "AGENTS.md"},
    {"path": ".claude/agents/reviewer.md", "target": ".claude/agents/reviewer.md"}
  ]
}
`
const testRecipe = `{
  "version": 1,
  "task": "Test the Lantern storage change using the testing checklist.",
  "sources": [
    {"path": "AGENTS.md", "target": "AGENTS.md"},
    {"path": ".claude/skills/testing/SKILL.md", "target": ".claude/skills/testing/SKILL.md"},
    {"path": ".claude/skills/testing/assets/checklist.txt", "target": ".claude/skills/testing/assets/checklist.txt"}
  ]
}
`

// Digests printed by `weave build` at github.com/hev/weave main (cb360fc),
// go1.27.1, for the example project and recipes. A port that changes any
// bundle byte changes these.
var weaveDigests = map[string]string{
	"review":   "f88dd154b5b84bf297be2c1c6553594988abc6d61562d4b2c433d1357add2c2c",
	"test":     "eeed41048939e990ae5ce56d2a60b42dd7966740f03a01baaa2e5fbe08da787b",
	"baseline": "e48b0cd85725494460e2bbeb479d57b2f8ecd8a9d10f09ab0018d86a196a9e2d", // extract's recipe.json
	"improved": "ef1b0a433c9903c06791f44d308e485b169014833a5c0e125c6c15050d09ca02", // extract's improved.recipe.json
}

func exampleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	for name, body := range exampleProject {
		p := filepath.Join(project, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"review.recipe.json": reviewRecipe, "test.recipe.json": testRecipe} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestWeaveDigestParity runs Weave's README/quickstart flow through the ported
// CLI and checks every bundle digest against Weave's own output.
func TestWeaveDigestParity(t *testing.T) {
	root := exampleRoot(t)
	project := filepath.Join(root, "project")
	run := func(args ...string) string {
		t.Helper()
		var w bytes.Buffer
		if err := Run(args, &w); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return w.String()
	}
	run("assess", "--root", project, "--out", filepath.Join(root, "assessment"))
	run("extract", "--root", project, "--out", filepath.Join(root, "extracted"))
	sources := filepath.Join(root, "extracted", "sources")
	builds := map[string][2]string{
		"review":   {sources, filepath.Join(root, "review.recipe.json")},
		"test":     {sources, filepath.Join(root, "test.recipe.json")},
		"baseline": {project, filepath.Join(root, "extracted", "recipe.json")},
		"improved": {project, filepath.Join(root, "extracted", "improved.recipe.json")},
	}
	for name, in := range builds {
		out := filepath.Join(root, "bundle-"+name)
		run("build", "--root", in[0], "--recipe", in[1], "--out", out)
		b, err := LoadBundle(out)
		if err != nil {
			t.Fatal(err)
		}
		if b.Digest != weaveDigests[name] {
			t.Errorf("%s digest %s, Weave produced %s", name, b.Digest, weaveDigests[name])
		}
	}
	// Building straight from the project gives the same bundle as from extract.
	run("build", "--root", project, "--recipe", filepath.Join(root, "review.recipe.json"), "--out", filepath.Join(root, "direct"))
	if b, _ := LoadBundle(filepath.Join(root, "direct")); b.Digest != weaveDigests["review"] {
		t.Errorf("direct review digest %s", b.Digest)
	}
	// quickstart.sh: a repeat build is byte-identical, and diff succeeds.
	run("build", "--root", sources, "--recipe", filepath.Join(root, "test.recipe.json"), "--out", filepath.Join(root, "repeat"))
	a, _ := os.ReadFile(filepath.Join(root, "bundle-test", "bundle.json"))
	b, _ := os.ReadFile(filepath.Join(root, "repeat", "bundle.json"))
	if !bytes.Equal(a, b) {
		t.Fatal("repeat build differs")
	}
	if out := run("diff", "--before", filepath.Join(root, "bundle-review"), "--after", filepath.Join(root, "bundle-test"), "--out", filepath.Join(root, "comparison")); out == "" {
		t.Fatal("diff printed nothing")
	}
}

func TestDecodeRecipeStrict(t *testing.T) {
	if _, err := DecodeRecipe([]byte(testRecipe)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"version":1,"unknown":1}`, `{} {}`, `broken`} {
		if _, err := DecodeRecipe([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
