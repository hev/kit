package dispatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/skillsources"
)

// Shelf skills for one run (RFC 0018). Installed is not active: shelf packs
// sit in the pinned store until an order (skills:) or its context recipe
// enables some of their skills. The target resolves those names against its
// own shelf before the run is accepted, so an unknown skill or a second mode
// pack is refused at dispatch, and links exactly those skills into the
// worktree's project skill root for the harness (.claude/skills for Claude
// Code, .agents/skills for Codex and agy) for as long as the harness runs. Bundled
// kit skills and the operator's own skills in the home roots are untouched.
//
// The links are kept out of git by a .gitignore dispatch writes in the skill
// root (only when it has none of its own), and are removed, with anything
// dispatch created, when the harness exits; a supervisor that died leaves
// skills.json in the run record and the next attempt undoes it first.

const skillsState = "skills.json"

// RunSkill is one shelf skill a run enables, as recorded in meta.json.
type RunSkill = skillsources.Enabled

// harnessSkillRoot is the worktree skill directory a harness reads.
// agy (Antigravity) reads the shared .agents/skills root, as Codex does.
func harnessSkillRoot(harness string) string {
	if harness == "codex" || harness == "agy" {
		return filepath.Join(".agents", "skills")
	}
	return filepath.Join(".claude", "skills")
}

// shelfHome and shelfConfig locate the target's shelf; tests replace them.
var (
	shelfHome   = os.UserHomeDir
	shelfConfig = layerenv.ConfigPath
)

// runSkillNames is the order's skills followed by the recipe's, without
// repeats.
func runSkillNames(m Meta) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range append(append([]string{}, m.Skills...), m.ContextSkills...) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// prepareSkills resolves this attempt's shelf skills on the target and records
// them on m. A name the target cannot resolve, or more than one mode pack,
// refuses the attempt.
func prepareSkills(dir string, m *Meta) error {
	m.ShelfSkills = nil
	if m.Parent != "" {
		return nil
	}
	if leftover := restoreSkills(dir); len(leftover) > 0 {
		return fmt.Errorf("shelf skills from an earlier attempt could not be removed: %s", strings.Join(leftover, "; "))
	}
	enabled, err := resolveSkills(runSkillNames(*m))
	if err != nil {
		return err
	}
	m.ShelfSkills = enabled
	return nil
}

// resolveSkills checks names against this host's shelf: each must be an
// installed pack's skill, and at most one mode pack may appear.
func resolveSkills(names []string) ([]RunSkill, error) {
	if len(names) == 0 {
		return nil, nil
	}
	home, err := shelfHome()
	if err != nil {
		return nil, err
	}
	cfg, err := skillsources.Load(shelfConfig())
	if err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	enabled, err := skillsources.Enable(home, cfg, names)
	if err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	return enabled, nil
}

type skillsRecord struct {
	// Created lists what dispatch made in the worktree, in creation order.
	Created []string `json:"created"`
}

// applySkills links the attempt's shelf skills into the worktree and returns
// the function that removes them again.
func applySkills(dir string, m Meta) (func() []string, error) {
	noop := func() []string { return nil }
	if len(m.ShelfSkills) == 0 {
		return noop, nil
	}
	if leftover := restoreSkills(dir); len(leftover) > 0 {
		return noop, fmt.Errorf("shelf skills: %s", strings.Join(leftover, "; "))
	}
	wt := filepath.Join(dir, "wt")
	rel := harnessSkillRoot(m.Harness)
	root := filepath.Join(wt, rel)
	var rec skillsRecord
	save := func() error { return WriteJSON(filepath.Join(dir, skillsState), rec) }
	undo := func() []string { return restoreSkills(dir) }
	// Record each path before it is made, so a crash leaves nothing unlisted.
	var missing []string
	for p := root; p != wt; p = filepath.Dir(p) {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			missing = append([]string{p}, missing...)
		} else if err != nil {
			return noop, err
		} else if fi, _ := os.Lstat(p); fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return noop, fmt.Errorf("shelf skills: %s is not a plain directory", p)
		}
	}
	for _, p := range missing {
		rec.Created = append(rec.Created, p)
		if err := save(); err != nil {
			return noop, err
		}
		if err := os.Mkdir(p, 0755); err != nil {
			undo()
			return noop, err
		}
	}
	ignore := filepath.Join(root, ".gitignore")
	ownRoot := len(missing) > 0
	if _, err := os.Lstat(ignore); err == nil {
		undo()
		return noop, fmt.Errorf("shelf skills: %s already has a .gitignore; dispatch cannot keep linked skills out of git there", filepath.ToSlash(rel))
	}
	var lines []string
	if ownRoot {
		lines = []string{"*"}
	} else {
		lines = []string{"/.gitignore"}
	}
	for _, s := range m.ShelfSkills {
		dest := filepath.Join(root, s.Skill)
		if _, err := os.Lstat(dest); err == nil {
			undo()
			return noop, fmt.Errorf("shelf skills: the worktree already has %s; %s would shadow it", filepath.ToSlash(filepath.Join(rel, s.Skill)), s.Name())
		}
		if !ownRoot {
			lines = append(lines, "/"+s.Skill)
		}
	}
	rec.Created = append(rec.Created, ignore)
	if err := save(); err != nil {
		undo()
		return noop, err
	}
	text := "# Written by hev dispatch for this run's shelf skills; removed when the harness exits.\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(ignore, []byte(text), 0644); err != nil {
		undo()
		return noop, err
	}
	for _, s := range m.ShelfSkills {
		dest := filepath.Join(root, s.Skill)
		rec.Created = append(rec.Created, dest)
		if err := save(); err != nil {
			undo()
			return noop, err
		}
		var err error
		if runtime.GOOS == "windows" {
			err = copySkillTree(s.Dir, dest)
		} else {
			err = os.Symlink(s.Dir, dest)
		}
		if err != nil {
			undo()
			return noop, fmt.Errorf("shelf skills: link %s: %w", s.Name(), err)
		}
	}
	return undo, nil
}

// restoreSkills removes what applySkills recorded, newest first, and the
// record itself once everything is gone. It never removes a directory that
// still holds something dispatch did not put there. Problems are returned.
func restoreSkills(dir string) []string {
	path := filepath.Join(dir, skillsState)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return []string{err.Error()}
	}
	var rec skillsRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	wt := filepath.Join(dir, "wt")
	var problems []string
	for i := len(rec.Created) - 1; i >= 0; i-- {
		p := rec.Created[i]
		if r, err := filepath.Rel(wt, p); err != nil || r == "." || strings.HasPrefix(r, "..") {
			problems = append(problems, "refusing to remove "+p+" outside the worktree")
			continue
		}
		fi, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0, !fi.IsDir():
			err = os.Remove(p)
		case i > 0 && filepath.Base(filepath.Dir(p)) == "skills" && isSkillCopy(rec, i):
			// A Windows copy of a pinned skill.
			err = os.RemoveAll(p)
		default:
			err = os.Remove(p)
			if err != nil && !os.IsNotExist(err) {
				if entries, _ := os.ReadDir(p); len(entries) > 0 {
					// Something else now lives here; leave it.
					err = nil
				}
			}
		}
		if err != nil && !os.IsNotExist(err) {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) == 0 {
		_ = os.Remove(path)
	}
	return problems
}

// isSkillCopy reports whether entry i is a linked skill (it follows the
// .gitignore entry) rather than a directory dispatch created.
func isSkillCopy(rec skillsRecord, i int) bool {
	for j := 0; j < i; j++ {
		if filepath.Base(rec.Created[j]) == ".gitignore" {
			return true
		}
	}
	return false
}

func copySkillTree(src, dest string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		to := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(to, 0755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file %s", p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(to, b, info.Mode().Perm())
	})
}

// skillsNote tells the worker which shelf skills the run enabled.
func skillsNote(m Meta) string {
	if len(m.ShelfSkills) == 0 {
		return ""
	}
	var names []string
	for _, s := range m.ShelfSkills {
		names = append(names, s.Name())
	}
	sort.Strings(names)
	return " This run enables shelf skills " + strings.Join(names, ", ") + " in " + filepath.ToSlash(harnessSkillRoot(m.Harness)) + "; they are linked for this run only and ignored by git."
}
