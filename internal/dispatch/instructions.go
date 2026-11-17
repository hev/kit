package dispatch

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/weave"
)

// A recipe-driven run's repository instruction files reach the harness in the
// form the recipe gives them. Harnesses read AGENTS.md, CLAUDE.md, agents and
// skills from the worktree themselves, so the supervisor puts each file in
// that form in place for exactly as long as the harness runs:
//
//   - narrowed: a covered file with keep/drop selectors holds the bundle's
//     narrowed bytes;
//   - whole: a covered file without selectors is left untouched;
//   - excluded: a discovered file the recipe does not cover is removed;
//   - full: a file that cannot be replaced safely (untracked, or with
//     uncommitted changes) is left as it is, and the record says so.
//
// Every replaced or removed file is marked skip-worktree in the worktree's own
// index first, so git status, diff, add -A, commit -a and stash do not see it
// and an explicit git add is refused. When the harness exits the supervisor
// clears the mark and checks the file out from the index again; a leftover
// from a supervisor that died is undone before the next attempt is built or
// launched. The state lives in the run record (instructions.json), never in
// the worktree.

const (
	FormNarrowed = "narrowed"
	FormWhole    = "whole"
	FormExcluded = "excluded"
	FormFull     = "full"

	// UncoveredRule is what happens to discovered instruction files a recipe
	// does not name.
	UncoveredRule = "excluded"
	// InPlaceMechanism names how replaced files are kept out of git.
	InPlaceMechanism = "skip-worktree"
)

// InstructionFile is one repository instruction, agent or skill file and the
// form the harness could load it in.
type InstructionFile struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Covered bool   `json:"covered"`
	Form    string `json:"form"`
	// SHA256 and Bytes describe what the harness could read: the narrowed
	// bytes, or the full file; empty for an excluded file.
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int    `json:"bytes"`
	// SourceSHA256 is the full file in the worktree when the bundle was built.
	SourceSHA256 string `json:"source_sha256"`
	SourceBytes  int    `json:"source_bytes"`
	Reason       string `json:"reason,omitempty"`
}

type appliedFile struct {
	Path   string `json:"path"`
	Form   string `json:"form"`
	SHA256 string `json:"sha256,omitempty"`
}

type appliedState struct {
	Bundle string        `json:"bundle"`
	Files  []appliedFile `json:"files"`
}

const instructionsState = "instructions.json"

// skillDir is the skill directory a path belongs to, or "".
func skillDir(p string) string {
	for _, root := range []string{".claude/skills/", ".agents/skills/"} {
		if rest, ok := strings.CutPrefix(p, root); ok {
			if name, _, ok := strings.Cut(rest, "/"); ok {
				return root + name + "/"
			}
		}
	}
	return ""
}

// gitIndex lists tracked paths with their ls-files -v tags, and the paths
// whose worktree copy differs from the index.
func gitIndex(wt string) (tags map[string]byte, dirty map[string]bool, err error) {
	out, err := gitOutput(wt, "ls-files", "-z", "-v")
	if err != nil {
		return nil, nil, err
	}
	tags = map[string]byte{}
	for _, e := range strings.Split(string(out), "\x00") {
		if len(e) > 2 {
			tags[e[2:]] = e[0]
		}
	}
	out, err = gitOutput(wt, "diff", "--name-only", "-z")
	if err != nil {
		return nil, nil, err
	}
	dirty = map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			dirty[p] = true
		}
	}
	return tags, dirty, nil
}

// planInstructions decides each discovered file's form for a recipe, before
// the bundle is built. Hashes of narrowed bytes are filled in afterwards.
func planInstructions(root string, recipe weave.Recipe) ([]InstructionFile, error) {
	found, _ := weave.Discover(root)
	covered := map[string]weave.Source{}
	for _, s := range recipe.Sources {
		if _, dup := covered[s.Path]; dup {
			covered[s.Path] = weave.Source{Path: s.Path, Keep: []string{"\x00dup"}}
			continue
		}
		covered[s.Path] = s
	}
	coveredSkills := map[string]bool{}
	for p := range covered {
		if d := skillDir(p); d != "" {
			coveredSkills[d] = true
		}
	}
	tags, dirty, err := gitIndex(root)
	if err != nil {
		return nil, err
	}
	files := []InstructionFile{}
	for _, d := range found.Sources {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(d.Path)))
		if err != nil {
			return nil, fmt.Errorf("instruction file %s: %w", d.Path, err)
		}
		f := InstructionFile{Path: d.Path, Kind: weave.SourceKind(d.Path), SourceSHA256: weave.Hash(data), SourceBytes: len(data)}
		s, ok := covered[d.Path]
		f.Covered = ok
		switch {
		case ok && len(s.Keep) == 1 && s.Keep[0] == "\x00dup":
			return nil, fmt.Errorf("instruction file %s is a source more than once; a harness can load only one form of it", d.Path)
		case ok && len(s.Keep) == 0 && len(s.Drop) == 0:
			f.Form = FormWhole
		case !ok && coveredSkills[skillDir(d.Path)]:
			f.Form, f.Reason = FormWhole, "part of a skill the recipe covers"
		default:
			f.Form = FormNarrowed
			if !ok {
				f.Form = FormExcluded
			}
			if tags[d.Path] == 0 {
				f.Form, f.Reason = FormFull, "untracked, so it cannot be replaced without risking a commit"
			} else if dirty[d.Path] || tags[d.Path] != 'H' {
				f.Form, f.Reason = FormFull, "has uncommitted changes in the worktree, left as they are"
			}
		}
		if f.Form != FormExcluded {
			f.SHA256, f.Bytes = f.SourceSHA256, f.SourceBytes
		}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// instructionNote tells the worker which instruction files are replaced in its
// worktree and how to edit one deliberately. It becomes part of the
// assignment, so it is in the bundle and its digest.
func instructionNote(files []InstructionFile) string {
	var lines []string
	for _, f := range files {
		switch f.Form {
		case FormNarrowed:
			lines = append(lines, "- "+f.Path+": narrowed to the sections this task needs")
		case FormExcluded:
			lines = append(lines, "- "+f.Path+": not present for this task")
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n\n## Instruction files in this worktree (from dispatch)\n\n" +
		"For this run dispatch replaced these files in the worktree, following the order's context recipe. Git is told to ignore them (skip-worktree), so they do not appear in git status or diffs and are never committed, and they are put back when you exit:\n\n" +
		strings.Join(lines, "\n") + "\n\n" +
		"If your task is to edit one of them, or a rebase or checkout stops on one, first restore the full file with `git update-index --no-skip-worktree -- PATH && git checkout -- PATH`, then work on it and commit it as usual."
}

// applyInstructions puts the attempt's narrowed and excluded files in place
// and returns a function that undoes it. It refuses to launch if a file
// changed since the bundle was built.
func applyInstructions(dir string, m Meta) (func() []string, error) {
	noop := func() []string { return nil }
	leftover := restoreInstructions(dir)
	rec := m.ContextBundle
	if m.Parent != "" || rec == nil {
		return func() []string { return leftover }, nil
	}
	wt := filepath.Join(dir, "wt")
	b, err := weave.LoadBundle(filepath.Join(dir, filepath.FromSlash(rec.Bundle)))
	if err != nil {
		return noop, fmt.Errorf("context bundle %s: %w", rec.Bundle, err)
	}
	narrowed := map[string][]byte{}
	for _, f := range b.Files {
		if f.Selection != nil {
			narrowed[f.Path] = f.Data
		}
	}
	tags, dirty, err := gitIndex(wt)
	if err != nil {
		return noop, err
	}
	state := appliedState{Bundle: rec.Bundle}
	for _, f := range rec.Instructions {
		if f.Form != FormNarrowed && f.Form != FormExcluded {
			continue
		}
		if err := weave.CheckPath(f.Path); err != nil {
			return noop, err
		}
		cur, err := os.ReadFile(filepath.Join(wt, filepath.FromSlash(f.Path)))
		if err != nil || weave.Hash(cur) != f.SourceSHA256 || tags[f.Path] != 'H' || dirty[f.Path] {
			return noop, fmt.Errorf("instruction file %s changed since the context bundle was built; rework to rebuild it", f.Path)
		}
		a := appliedFile{Path: f.Path, Form: f.Form}
		if f.Form == FormNarrowed {
			data, ok := narrowed[f.Path]
			if !ok || weave.Hash(data) != f.SHA256 {
				return noop, fmt.Errorf("context bundle %s has no narrowed %s matching the record", rec.Bundle, f.Path)
			}
			a.SHA256 = f.SHA256
		}
		state.Files = append(state.Files, a)
	}
	if len(state.Files) == 0 {
		return func() []string { return leftover }, nil
	}
	// Record first, so a supervisor that dies part way is still undone.
	if err := WriteJSON(filepath.Join(dir, instructionsState), state); err != nil {
		return noop, err
	}
	fail := func(err error) (func() []string, error) {
		restoreInstructions(dir)
		return noop, err
	}
	paths := []string{"update-index", "--skip-worktree", "--"}
	for _, a := range state.Files {
		paths = append(paths, a.Path)
	}
	if _, err := gitOutput(wt, paths...); err != nil {
		return fail(err)
	}
	for _, a := range state.Files {
		p := filepath.Join(wt, filepath.FromSlash(a.Path))
		if a.Form == FormExcluded {
			err = os.Remove(p)
		} else {
			err = os.WriteFile(p, narrowed[a.Path], 0644)
		}
		if err != nil {
			return fail(fmt.Errorf("instruction file %s: %w", a.Path, err))
		}
	}
	return func() []string { return append(leftover, restoreInstructions(dir)...) }, nil
}

// restoreInstructions undoes applyInstructions: it clears skip-worktree and
// checks each replaced file out from the index. A worker's change to a
// replaced file is saved in the run record, reported, and never committed; a
// file the worker restored and then edited on purpose is left alone.
func restoreInstructions(dir string) []string {
	statePath := filepath.Join(dir, instructionsState)
	var state appliedState
	if readJSON(statePath, &state) != nil {
		return nil
	}
	wt := filepath.Join(dir, "wt")
	tags, _, err := gitIndex(wt)
	if err != nil {
		return []string{"cannot restore instruction files: " + err.Error()}
	}
	var warnings []string
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, a := range state.Files {
		if weave.CheckPath(a.Path) != nil {
			continue
		}
		p := filepath.Join(wt, filepath.FromSlash(a.Path))
		cur, readErr := os.ReadFile(p)
		ours := (a.Form == FormExcluded && os.IsNotExist(readErr)) || (a.Form == FormNarrowed && readErr == nil && weave.Hash(cur) == a.SHA256)
		marked := tags[a.Path] == 'S'
		if marked {
			if _, err := gitOutput(wt, "update-index", "--no-skip-worktree", "--", a.Path); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", a.Path, err))
				continue
			}
		}
		index, indexErr := gitOutput(wt, "show", ":"+a.Path)
		switch {
		case ours:
			if !marked {
				warnings = append(warnings, a.Path+": skip-worktree was cleared without restoring the file; restored it from the index")
			}
		case !marked:
			continue // restored on purpose by the worker; its edits are its own
		case indexErr == nil && readErr == nil && bytes.Equal(cur, index):
			continue // git itself wrote the full file back (rebase, checkout)
		default:
			saved := filepath.Join("instruction-edits", stamp, filepath.FromSlash(a.Path))
			if readErr == nil {
				full := filepath.Join(dir, saved)
				if os.MkdirAll(filepath.Dir(full), 0700) == nil && os.WriteFile(full, cur, 0600) == nil {
					warnings = append(warnings, fmt.Sprintf("%s: changed while it was %s, so git ignored the change; saved to %s and restored the file", a.Path, a.Form, filepath.ToSlash(saved)))
					break
				}
			}
			warnings = append(warnings, fmt.Sprintf("%s: changed or removed while it was %s; restored the file", a.Path, a.Form))
		}
		if indexErr != nil {
			continue // no longer in the index; nothing to check out
		}
		if _, err := gitOutput(wt, "checkout", "--", a.Path); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", a.Path, err))
		}
	}
	_ = os.Remove(statePath)
	return warnings
}
