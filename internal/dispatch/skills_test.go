package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/skillsources"
)

func TestOrderSkillsFrontmatter(t *testing.T) {
	base := "---\nname: s1\nrepo: kit\n%s---\n\n## Acceptance\n- ok\n"
	for in, want := range map[string]string{
		"skills: [pstack/poteto-mode, hunk/hunk-review]\n": "pstack/poteto-mode,hunk/hunk-review",
		"skills: [\"pstack/poteto-mode\"] # one mode\n":    "pstack/poteto-mode",
		"skills: hunk/hunk-review\n":                       "hunk/hunk-review",
		"skills: []\n":                                     "",
		"":                                                 "",
	} {
		o, err := Parse(strings.Replace(base, "%s", in, 1))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got := strings.Join(o.Skills, ","); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"skills: [pstack]\n", "skills: [a/b, a/b]\n", "skills: [a/b\n", "skills: [A/b]\n", "skills: [a/b/c]\n"} {
		if _, err := Parse(strings.Replace(base, "%s", bad, 1)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func shelfGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
	return strings.TrimSpace(string(b))
}

// shelfHost installs two packs, a mode and a tool, in a temporary home and
// points the target's shelf lookups at it.
func shelfHost(t *testing.T) {
	t.Helper()
	up := t.TempDir()
	shelfGit(t, up, "init", "-q", "-b", "main")
	write := func(p, s string) {
		os.MkdirAll(filepath.Dir(filepath.Join(up, p)), 0755)
		if err := os.WriteFile(filepath.Join(up, p), []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"mode/skills/drive", "mode/skills/steer", "other-mode/skills/flow", "tools/skills/review", "tools/skills/grill"} {
		write(s+"/SKILL.md", "---\nname: "+filepath.Base(s)+"\n---\nbody\n")
	}
	write("LICENSE", "MIT License\nPermission is hereby granted, free of charge, to any person\nThe above copyright notice and this permission notice shall be included\n")
	shelfGit(t, up, "add", ".")
	shelfGit(t, up, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "packs")
	sha := shelfGit(t, up, "rev-parse", "HEAD")
	home := t.TempDir()
	config := filepath.Join(t.TempDir(), "config.toml")
	text := ""
	for _, p := range []struct{ name, kind, sub string }{{"steering", "mode", "mode"}, {"rival", "mode", "other-mode"}, {"kit-tools", "tool", "tools"}} {
		text += "[[skills.sources]]\nname = \"" + p.name + "\"\nkind = \"" + p.kind + "\"\npath = \"" + filepath.ToSlash(up) + "\"\nsubdir = \"" + p.sub + "\"\ncommit = \"" + sha + "\"\n\n"
	}
	if err := os.WriteFile(config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := skillsources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := skillsources.Run(home, cfg, true, nil); err != nil {
		t.Fatal(err)
	}
	oldHome, oldConfig := shelfHome, shelfConfig
	shelfHome = func() (string, error) { return home, nil }
	shelfConfig = func() string { return config }
	t.Cleanup(func() { shelfHome, shelfConfig = oldHome, oldConfig })
}

func runWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wt := filepath.Join(dir, "wt")
	os.MkdirAll(wt, 0755)
	shelfGit(t, wt, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(wt, "README.md"), []byte("hi\n"), 0644)
	shelfGit(t, wt, "add", ".")
	shelfGit(t, wt, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "init")
	return dir
}

func TestRunEnablesOnlyNamedSkillsAndCleansUp(t *testing.T) {
	shelfHost(t)
	for _, harness := range []string{"claude", "codex"} {
		dir := runWorktree(t)
		wt := filepath.Join(dir, "wt")
		m := Meta{Order: Order{Name: "s1", Harness: harness, Skills: []string{"steering/drive", "kit-tools/review"}}}
		if err := prepareSkills(dir, &m); err != nil {
			t.Fatal(err)
		}
		if len(m.ShelfSkills) != 2 || m.ShelfSkills[0].Kind != "mode" || m.ShelfSkills[1].License != "MIT" {
			t.Fatalf("resolved %+v", m.ShelfSkills)
		}
		undo, err := applySkills(dir, m)
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(wt, harnessSkillRoot(harness))
		entries, _ := os.ReadDir(root)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if strings.Join(names, ",") != ".gitignore,drive,review" {
			t.Fatalf("%s linked %v", harness, names)
		}
		if b, err := os.ReadFile(filepath.Join(root, "drive", "SKILL.md")); err != nil || !strings.Contains(string(b), "name: drive") {
			t.Fatalf("drive: %q %v", b, err)
		}
		if runtime.GOOS != "windows" {
			if target, _ := os.Readlink(filepath.Join(root, "review")); skillsources.PinnedCommit(target) == "" {
				t.Fatalf("link %s is not into the pinned store", target)
			}
		}
		if st := shelfGit(t, wt, "status", "--porcelain"); st != "" {
			t.Fatalf("linked skills visible to git:\n%s", st)
		}
		if problems := undo(); len(problems) > 0 {
			t.Fatal(problems)
		}
		if _, err := os.Lstat(filepath.Join(wt, strings.Split(harnessSkillRoot(harness), string(filepath.Separator))[0])); !os.IsNotExist(err) {
			t.Fatalf("%s: dispatch left its skill root behind", harness)
		}
		if _, err := os.Stat(filepath.Join(dir, skillsState)); !os.IsNotExist(err) {
			t.Fatal("record left after cleanup")
		}
	}
}

func TestRunSkillsKeepRepositorySkillRoot(t *testing.T) {
	shelfHost(t)
	dir := runWorktree(t)
	wt := filepath.Join(dir, "wt")
	own := filepath.Join(wt, ".claude", "skills", "house")
	os.MkdirAll(own, 0755)
	os.WriteFile(filepath.Join(own, "SKILL.md"), []byte("house\n"), 0644)
	shelfGit(t, wt, "add", ".")
	shelfGit(t, wt, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "house skill")
	m := Meta{Order: Order{Name: "s1", Harness: "claude", Skills: []string{"kit-tools/grill"}}}
	if err := prepareSkills(dir, &m); err != nil {
		t.Fatal(err)
	}
	undo, err := applySkills(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	if st := shelfGit(t, wt, "status", "--porcelain"); st != "" {
		t.Fatalf("git sees %s", st)
	}
	// A leftover from a supervisor that died is undone before the next attempt.
	if err := prepareSkills(dir, &m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(wt, ".claude", "skills", "grill")); !os.IsNotExist(err) {
		t.Fatal("leftover link survived the next attempt's preparation")
	}
	undo()
	if b, err := os.ReadFile(filepath.Join(own, "SKILL.md")); err != nil || string(b) != "house\n" {
		t.Fatal("repository skill touched")
	}
	entries, _ := os.ReadDir(filepath.Join(wt, ".claude", "skills"))
	if len(entries) != 1 {
		t.Fatalf("left %v", entries)
	}
	// A shelf skill may not shadow a repository skill of the same name.
	os.MkdirAll(filepath.Join(wt, ".claude", "skills", "review"), 0755)
	m.Skills = []string{"kit-tools/review"}
	if err := prepareSkills(dir, &m); err != nil {
		t.Fatal(err)
	}
	if _, err := applySkills(dir, m); err == nil || !strings.Contains(err.Error(), "shadow") {
		t.Fatalf("shadowing: %v", err)
	}
}

func TestRunSkillsRefusals(t *testing.T) {
	shelfHost(t)
	dir := runWorktree(t)
	m := Meta{Order: Order{Name: "s1", Harness: "claude", Skills: []string{"steering/drive"}}, ContextSkills: []string{"rival/flow"}}
	if err := prepareSkills(dir, &m); err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("two modes from order and recipe: %v", err)
	}
	m = Meta{Order: Order{Name: "s1", Harness: "claude", Skills: []string{"kit-tools/missing"}}}
	if err := prepareSkills(dir, &m); err == nil {
		t.Fatal("unknown skill accepted")
	}
	// No skills: nothing resolved, nothing linked.
	m = Meta{Order: Order{Name: "s1", Harness: "claude"}}
	if err := prepareSkills(dir, &m); err != nil || m.ShelfSkills != nil {
		t.Fatalf("%v %v", err, m.ShelfSkills)
	}
	undo, err := applySkills(dir, m)
	if err != nil || len(undo()) != 0 {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "wt", ".claude")); !os.IsNotExist(err) {
		t.Fatal("a run without skills got a skill root")
	}
	// Validators never get shelf skills.
	m = Meta{Order: Order{Name: "s1", Harness: "claude", Skills: []string{"steering/drive"}}, Parent: "p"}
	if err := prepareSkills(dir, &m); err != nil || m.ShelfSkills != nil {
		t.Fatalf("validator: %v %v", err, m.ShelfSkills)
	}
}

// Through the supervisor: the harness sees exactly the enabled shelf skills
// beside the repository's own, and they are gone when it exits.
func TestSupervisorLinksShelfSkillsForTheRunOnly(t *testing.T) {
	allowHarnessProbes(t)
	h := newContextHarness(t)
	shelfHost(t)
	order := strings.Replace(contextOrder("shelf-run", "claude", ""), "---\nAdd", "skills: [steering/drive, kit-tools/review]\n---\nAdd", 1)
	if err := h.start(order); err != nil {
		t.Fatal(err)
	}
	m := runMeta(t, "shelf-run")
	if len(m.ShelfSkills) != 2 || m.ShelfSkills[0].Name() != "steering/drive" || m.ShelfSkills[0].Commit == "" {
		t.Fatalf("meta %+v", m.ShelfSkills)
	}
	dir := filepath.Join(Root(), "shelf-run")
	wt := filepath.Join(dir, "wt")
	if err := superviseRound("shelf-run"); err != nil {
		t.Fatal(err)
	}
	seen := h.seen(1, "claude")
	for _, want := range []string{".claude/skills/drive", ".claude/skills/review", ".claude/skills/fix/SKILL.md"} {
		if _, ok := seen[want]; !ok {
			t.Fatalf("harness did not see %s: %v", want, seen)
		}
	}
	for p := range seen {
		if strings.Contains(p, "steer") || strings.Contains(p, "grill") || strings.Contains(p, "flow") {
			t.Fatalf("harness saw a skill the run did not enable: %s", p)
		}
	}
	_, args := h.captured(1, "claude")
	if !strings.Contains(strings.Join(args, " "), "enables shelf skills kit-tools/review, steering/drive") {
		t.Fatalf("prompt does not name the skills: %v", args)
	}
	r := ctxResult(t, dir)
	if r.Error != "" || len(r.InstructionWarnings) != 0 {
		t.Fatalf("result %+v", r)
	}
	for _, p := range []string{"drive", "review", ".gitignore"} {
		if _, err := os.Lstat(filepath.Join(wt, ".claude", "skills", p)); !os.IsNotExist(err) {
			t.Fatalf("%s left after the run", p)
		}
	}
	if st := h.git(wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("worktree not clean: %s", st)
	}
	if _, err := os.Stat(filepath.Join(dir, skillsState)); !os.IsNotExist(err) {
		t.Fatal("skills record left")
	}
	// Two mode packs are refused before anything runs.
	two := strings.Replace(contextOrder("shelf-two", "claude", ""), "---\nAdd", "skills: [steering/drive, rival/flow]\n---\nAdd", 1)
	if err := h.start(two); err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("two modes: %v", err)
	}
}
