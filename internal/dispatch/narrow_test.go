package dispatch

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/weave"
)

// The narrowing fixture: two instruction files with sections a task does not
// need, a skill the recipe leaves out and one it keeps.
var narrowFixture = map[string]string{
	"AGENTS.md":                      "# Project\nScope: all tasks.\n\n## Build\nRun make.\n\n## Release\nRELEASE-STEPS are long.\n\n## Style\nKeep it short.\n",
	"CLAUDE.md":                      "# Claude\nIntro.\n## Deploy\nDEPLOY-SECRETS-HOWTO\n## Notes\nUse the widget skill.\n",
	".claude/skills/deploy/SKILL.md": "---\nname: deploy\n---\nScope: deploys.\nDEPLOY-SKILL\n",
	".claude/skills/deploy/run.sh":   "echo deploy\n",
	".claude/skills/fix/SKILL.md":    "---\nname: fix\n---\nScope: fixing.\nFix carefully.\n",
	"widget.txt":                     "widget v1\n",
}

const narrowRecipe = `{"version":1,"task":"replaced","sources":[` +
	`{"path":"AGENTS.md","target":"AGENTS.md","drop":["## Release"]},` +
	`{"path":"CLAUDE.md","target":"CLAUDE.md","keep":["## Notes"]},` +
	`{"path":".claude/skills/fix/SKILL.md","target":".claude/skills/fix/SKILL.md"}]}` + "\n"

func sum256(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

// newNarrowHarness replaces the context fixture's files with the narrowing
// fixture and commits a copy of the recipe for committed-recipe runs.
func newNarrowHarness(t *testing.T) *contextHarness {
	h := newContextHarness(t)
	h.git(h.origin, "rm", "-q", "-r", ".")
	files := map[string]string{"recipes/narrow.json": narrowRecipe}
	for k, v := range narrowFixture {
		files[k] = v
	}
	for name, body := range files {
		p := filepath.Join(h.origin, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h.git(h.origin, "-c", "core.autocrlf=false", "add", ".")
	h.git(h.origin, "commit", "-q", "-m", "narrow fixture")
	h.git(filepath.Join(h.workspace, "proj"), "pull", "-q", "--ff-only")
	return h
}

// carriedOrder is an order with the recipe carried in its frontmatter, as
// hev dispatch --context-file writes it.
func carriedOrder(t *testing.T, name, harness, recipe string) string {
	t.Helper()
	text, err := WithContextRecipe(contextOrder(name, harness, ""), []byte(recipe))
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// seen parses a harness's report of the instruction files it could read.
func (h *contextHarness) seen(n int, name string) map[string]string {
	h.t.Helper()
	h.captured(n, name)
	raw, err := os.ReadFile(filepath.Join(h.capture, itoa(n)+"-"+name+".files"))
	if err != nil {
		h.t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if p, sha, ok := strings.Cut(line, " "); ok {
			out[p] = sha
		}
	}
	return out
}

func (h *contextHarness) actions(lines ...string) {
	h.t.Helper()
	p := filepath.Join(h.t.TempDir(), "actions")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		h.t.Fatal(err)
	}
	h.t.Setenv("CONTEXT_ACTIONS", p)
}

// checkNarrowedView asserts what a harness could read: the narrowed files,
// the whole skill, and nothing of the excluded skill.
func checkNarrowedView(t *testing.T, seen map[string]string, rec *ContextRecord) {
	t.Helper()
	agents, _, err := weave.Narrow([]byte(narrowFixture["AGENTS.md"]), nil, []string{"## Release"})
	if err != nil {
		t.Fatal(err)
	}
	claude, _, _ := weave.Narrow([]byte(narrowFixture["CLAUDE.md"]), []string{"## Notes"}, nil)
	want := map[string]string{
		"AGENTS.md":                   sum256(string(agents)),
		"CLAUDE.md":                   sum256(string(claude)),
		".claude/skills/fix/SKILL.md": sum256(narrowFixture[".claude/skills/fix/SKILL.md"]),
	}
	if len(seen) != len(want) {
		t.Fatalf("harness could read %v, want exactly %v", seen, want)
	}
	for p, sha := range want {
		if seen[p] != sha {
			t.Fatalf("harness read %s as %s, want %s", p, seen[p], sha)
		}
	}
	// The record says the same thing the harness saw.
	forms := map[string]string{}
	for _, f := range rec.Instructions {
		forms[f.Path] = f.Form
		if f.Form != FormExcluded && f.SHA256 != want[f.Path] {
			t.Fatalf("record %s sha %s, harness read %s", f.Path, f.SHA256, want[f.Path])
		}
	}
	if forms["AGENTS.md"] != FormNarrowed || forms["CLAUDE.md"] != FormNarrowed || forms[".claude/skills/fix/SKILL.md"] != FormWhole ||
		forms[".claude/skills/deploy/SKILL.md"] != FormExcluded || forms[".claude/skills/deploy/run.sh"] != FormExcluded || len(forms) != 5 {
		t.Fatalf("recorded forms %v", forms)
	}
	if strings.Join(rec.HarnessAlsoLoads, ",") != ".claude/skills/fix/SKILL.md,AGENTS.md,CLAUDE.md" || rec.Uncovered != UncoveredRule || rec.InPlace != InPlaceMechanism {
		t.Fatalf("record %+v", rec)
	}
}

// checkRestored asserts the worktree is back to its committed state: full
// files, no skip-worktree marks, nothing for git to report, no state file.
func checkRestored(t *testing.T, h *contextHarness, dir string) {
	t.Helper()
	wt := filepath.Join(dir, "wt")
	if status := h.git(wt, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Fatalf("worktree not clean after the run: %s", status)
	}
	for _, line := range strings.Split(h.git(wt, "ls-files", "-v"), "\n") {
		if !strings.HasPrefix(line, "H ") {
			t.Fatalf("index entry still marked: %q", line)
		}
	}
	for _, p := range []string{"AGENTS.md", ".claude/skills/deploy/SKILL.md"} {
		b, err := os.ReadFile(filepath.Join(wt, filepath.FromSlash(p)))
		if err != nil || string(b) != h.git(wt, "show", "HEAD:"+p)+"\n" {
			t.Fatalf("%s not restored: %q %v", p, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, instructionsState)); !os.IsNotExist(err) {
		t.Fatalf("state file left: %v", err)
	}
}

func TestNarrowedInstructionsLifecycle(t *testing.T) {
	h := newNarrowHarness(t)
	launch := 0

	// A recipe carried in the order: pinned in the run record with its hash;
	// the sender's forged hash and bundle are discarded.
	text := carriedOrder(t, "narrow-claude", "claude", narrowRecipe)
	if err := h.start(text); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "narrow-claude")
	wt := filepath.Join(dir, "wt")
	m := runMeta(t, "narrow-claude")
	rec := m.ContextBundle
	if rec == nil || rec.Origin != "order" || rec.Recipe != pinnedRecipe || rec.RecipeCommit != "" || rec.RecipeSHA256 != weave.Hash([]byte(narrowRecipe)) || m.ContextRecipeSHA256 != rec.RecipeSHA256 {
		t.Fatalf("carried recipe record %+v (meta hash %s)", rec, m.ContextRecipeSHA256)
	}
	if pinned, err := os.ReadFile(filepath.Join(dir, pinnedRecipe)); err != nil || string(pinned) != narrowRecipe {
		t.Fatalf("pinned recipe %q %v", pinned, err)
	}
	if order, _ := os.ReadFile(filepath.Join(dir, "order.md")); !strings.Contains(string(order), "context_recipe: "+base64.StdEncoding.EncodeToString([]byte(narrowRecipe))) {
		t.Fatalf("order.md does not carry the recipe: %s", order)
	}
	for _, s := range rec.Sources {
		if s.Path == "AGENTS.md" && (s.SourceSHA256 != sum256(narrowFixture["AGENTS.md"]) || strings.Join(s.Dropped, ",") != "## Release" || len(s.Kept) != 3) {
			t.Fatalf("narrowed source record %+v", s)
		}
	}
	// Nothing is replaced until the harness starts.
	checkRestored(t, h, dir)

	// Round 1: the harness sees only the narrowed forms; a worker that
	// stages everything and commits commits only its own change.
	h.actions("append widget.txt widget v2", "git add -A", "git commit -q -am work", "git status --porcelain")
	if err := superviseRound("narrow-claude"); err != nil {
		t.Fatal(err)
	}
	launch++
	checkNarrowedView(t, h.seen(launch, "claude"), rec)
	stdin, _ := h.captured(launch, "claude")
	if !strings.Contains(string(stdin), "## Instruction files in this worktree (from dispatch)") || !strings.Contains(string(stdin), "git update-index --no-skip-worktree -- PATH && git checkout -- PATH") || strings.Contains(string(stdin), "RELEASE-STEPS") || strings.Contains(string(stdin), "DEPLOY-SECRETS") {
		t.Fatalf("stdin %q", stdin)
	}
	log, _ := os.ReadFile(filepath.Join(h.capture, itoa(launch)+"-claude.actions"))
	if !strings.HasPrefix(string(log), "status-before:\n$") {
		t.Fatalf("worker saw replaced files in git status: %s", log)
	}
	if files := h.git(wt, "show", "--name-only", "--format=", "HEAD"); files != "widget.txt" {
		t.Fatalf("commit touched %q\n%s", files, log)
	}
	if diff := h.git(wt, "diff", "origin/main", "--name-only"); diff != "widget.txt" {
		t.Fatalf("branch diff %q", diff)
	}
	r := ctxResult(t, dir)
	if r.Error != "" || len(r.InstructionWarnings) != 0 {
		t.Fatalf("result %+v", r)
	}
	checkRestored(t, h, dir)

	// The worker cannot change its next attempt's recipe through the
	// worktree or ../order.md: the pinned copy is used.
	other := strings.Replace(narrowRecipe, `"drop":["## Release"]`, `"drop":["## Style"]`, 1)
	if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte(carriedOrder(t, "narrow-claude", "claude", other)), 0600); err != nil {
		t.Fatal(err)
	}

	// Round 2 (rework): an accidental edit of a narrowed file is invisible
	// to git, saved in the run record, reported and undone.
	if _, err := Handle(Request{Action: "rework", Name: "narrow-claude", Note: "Polish."}); err != nil {
		t.Fatal(err)
	}
	m = runMeta(t, "narrow-claude")
	if m.ContextBundle.RecipeSHA256 != rec.RecipeSHA256 || m.ContextBundle.Bundle != "contexts/2" {
		t.Fatalf("rework did not use the pinned recipe: %+v", m.ContextBundle)
	}
	h.actions("append AGENTS.md OOPS-EDIT", "git commit -q -am oops")
	if err := superviseRound("narrow-claude"); err != nil {
		t.Fatal(err)
	}
	launch++
	checkNarrowedView(t, h.seen(launch, "claude"), m.ContextBundle)
	r = ctxResult(t, dir)
	if len(r.InstructionWarnings) != 1 || !strings.Contains(r.InstructionWarnings[0], "AGENTS.md: changed while it was narrowed") {
		t.Fatalf("warnings %v", r.InstructionWarnings)
	}
	saved, _ := filepath.Glob(filepath.Join(dir, "instruction-edits", "*", "AGENTS.md"))
	if len(saved) != 1 {
		t.Fatalf("edit not saved: %v", saved)
	}
	if b, _ := os.ReadFile(saved[0]); !strings.Contains(string(b), "OOPS-EDIT") {
		t.Fatalf("saved edit %q", b)
	}
	if h.git(wt, "log", "-1", "--format=%s") == "oops" {
		t.Fatal("an ignored edit was committed")
	}
	checkRestored(t, h, dir)

	// Round 3 (retry): the order calls for editing CLAUDE.md, so the worker
	// restores it as the note says, edits the full file and commits it.
	if _, err := Handle(Request{Action: "retry", Name: "narrow-claude"}); err != nil {
		t.Fatal(err)
	}
	h.actions("git update-index --no-skip-worktree -- CLAUDE.md", "git checkout -- CLAUDE.md", "append CLAUDE.md NEW-RULE", "git commit -q -am edit-claude")
	if err := superviseRound("narrow-claude"); err != nil {
		t.Fatal(err)
	}
	launch++
	h.seen(launch, "claude")
	if r = ctxResult(t, dir); len(r.InstructionWarnings) != 0 {
		t.Fatalf("deliberate edit warned: %v", r.InstructionWarnings)
	}
	committed := h.git(wt, "show", "HEAD:CLAUDE.md")
	if committed != strings.TrimSuffix(narrowFixture["CLAUDE.md"], "\n")+"\nNEW-RULE" {
		t.Fatalf("committed CLAUDE.md %q", committed)
	}
	if files := h.git(wt, "show", "--name-only", "--format=", "HEAD"); files != "CLAUDE.md" {
		t.Fatalf("edit commit touched %q", files)
	}
	checkRestored(t, h, dir)

	// A supervisor that died with files replaced: the next attempt puts them
	// back before building, so its sources are the full files.
	m = runMeta(t, "narrow-claude")
	if _, err := Handle(Request{Action: "rework", Name: "narrow-claude", Note: "Again."}); err != nil {
		t.Fatal(err)
	}
	m = runMeta(t, "narrow-claude")
	if _, err := applyInstructions(dir, m); err != nil {
		t.Fatal(err)
	}
	if status := h.git(wt, "status", "--porcelain"); status != "" {
		t.Fatalf("replaced files visible to git: %s", status)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "AGENTS.md")); strings.Contains(string(b), "RELEASE-STEPS") {
		t.Fatal("AGENTS.md was not replaced")
	}

	// Amend to codex: the amendment re-pins from the amended order and the
	// leftover is undone first; the codex launch path sees the same view.
	if _, err := Handle(Request{Action: "amend", Name: "narrow-claude", Order: carriedOrder(t, "narrow-claude", "codex", narrowRecipe)}); err != nil {
		t.Fatal(err)
	}
	m = runMeta(t, "narrow-claude")
	for _, s := range m.ContextBundle.Sources {
		if s.Path == "CLAUDE.md" && s.SourceSHA256 != sum256(h.git(wt, "show", "HEAD:CLAUDE.md")+"\n") {
			t.Fatalf("amend built from a replaced file: %+v", s)
		}
	}
	h.actions("git status --porcelain")
	if err := superviseRound("narrow-claude"); err != nil {
		t.Fatal(err)
	}
	launch++
	seen := h.seen(launch, "codex")
	if seen["CLAUDE.md"] == "" || seen["AGENTS.md"] != m.ContextBundle.Instructions[3].SHA256 || seen[".claude/skills/deploy/SKILL.md"] != "" {
		t.Fatalf("codex view %v (record %+v)", seen, m.ContextBundle.Instructions)
	}
	checkRestored(t, h, dir)

	// A fresh codex run with the same recipe committed in the repository.
	if err := h.start(contextOrder("narrow-codex", "codex", "recipes/narrow.json")); err != nil {
		t.Fatal(err)
	}
	m = runMeta(t, "narrow-codex")
	if m.ContextBundle.Origin != "" || m.ContextRecipeSHA256 != "" {
		t.Fatalf("committed recipe record %+v", m.ContextBundle)
	}
	h.actions("git status --porcelain")
	if err := superviseRound("narrow-codex"); err != nil {
		t.Fatal(err)
	}
	launch++
	checkNarrowedView(t, h.seen(launch, "codex"), m.ContextBundle)
	checkRestored(t, h, filepath.Join(Root(), "narrow-codex"))
}

func TestCarriedRecipeIsChecked(t *testing.T) {
	base := contextOrder("x", "claude", "")
	for _, bad := range []string{`{"version":1,"task":"t","sources":[],"digest":"forged"}`, `{"version":1,`, ""} {
		if _, err := WithContextRecipe(base, []byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := WithContextRecipe(contextOrder("x", "claude", "r.json"), []byte(narrowRecipe)); err == nil || !strings.Contains(err.Error(), "already names") {
		t.Fatalf("both recipe kinds: %v", err)
	}
	for _, v := range []string{"not base64!", base64.StdEncoding.EncodeToString([]byte(`{"version":1,"bundle":{}}`))} {
		if _, err := Parse("---\nname: x\nrepo: r\ncontext_recipe: " + v + "\n---\n## Acceptance\n- x\n"); err == nil || !strings.Contains(err.Error(), "invalid context_recipe") {
			t.Errorf("parsed %q: %v", v, err)
		}
	}
	text, err := WithContextRecipe(base, []byte(narrowRecipe))
	if err != nil {
		t.Fatal(err)
	}
	o, err := Parse(text)
	if err != nil || o.ContextRecipe == "" || o.Context != "" || orderIntent(text) != orderIntent(base) {
		t.Fatalf("%+v %v", o, err)
	}
	if !carriedRecipeChanged(Meta{}, o) || carriedRecipeChanged(Meta{ContextRecipeSHA256: weave.Hash([]byte(narrowRecipe))}, o) {
		t.Fatal("carried recipe change detection")
	}
	if err := remoteError(Response{Error: "unknown key: context_recipe", Version: "0.9.0"}); err == nil || !strings.Contains(err.Error(), "does not support context recipes") {
		t.Fatalf("%v", err)
	}
}

// A selector that matches nothing fails the run before launch.
func TestNarrowSelectorErrorFailsAdmission(t *testing.T) {
	h := newNarrowHarness(t)
	bad := strings.Replace(narrowRecipe, `"## Release"`, `"## Releases"`, 1)
	err := h.start(carriedOrder(t, "narrow-bad", "claude", bad))
	if err == nil || !strings.Contains(err.Error(), `"## Releases" matches no heading`) {
		t.Fatalf("%v", err)
	}
	if len(h.launched) != 0 {
		t.Fatalf("launched %v", h.launched)
	}
}
