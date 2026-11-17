package skillsources

// The skills shelf (RFC 0018): a small, curated set of upstream skill packs,
// each a source pinned to one reviewed commit. A pack is installed to the
// pinned store but never linked into a harness skill root by sync; a run
// enables named skills from it (Enable), and dispatch links exactly those
// into that run's worktree for as long as the harness runs.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Pack kinds. A mode owns the workflow, so a run takes at most one; tools and
// domain skills compose.
const (
	KindMode   = "mode"
	KindTool   = "tool"
	KindDomain = "domain"
)

// Pack is one installed shelf pack as the manifest records it.
type Pack struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	URL    string `json:"url,omitempty"`
	Path   string `json:"path,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Subdir string `json:"subdir,omitempty"`
	Commit string `json:"commit"`
	// Pin is the pinned store directory holding the pack at Commit.
	Pin string `json:"pin"`
	// License is the pack's SPDX id: the nearest licence file from the
	// pack's directory up to the repository root, else the one licence all
	// its skills share, else "mixed".
	License string  `json:"license"`
	Author  string  `json:"author,omitempty"`
	Home    string  `json:"homepage,omitempty"`
	Skills  []Skill `json:"skills"`
}

// Skill is one skill in a pack. Path is its directory relative to the pin,
// slash-separated.
type Skill struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	License string `json:"license"`
}

var (
	packName  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	skillName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	fullSHA   = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// Shelf reports whether the source is a shelf pack rather than a team source.
func (s Source) Shelf() bool { return s.Kind != "" || s.Commit != "" || s.Name != "" }

func (s Source) validateShelf() error {
	if !s.Shelf() {
		return nil
	}
	if !packName.MatchString(s.Name) {
		return fmt.Errorf("shelf pack needs a name of lowercase letters, digits, '.', '_' or '-', got %q", s.Name)
	}
	switch s.Kind {
	case KindMode, KindTool, KindDomain:
	default:
		return fmt.Errorf("pack %s: kind must be mode, tool or domain, got %q", s.Name, s.Kind)
	}
	if !fullSHA.MatchString(s.Commit) {
		return fmt.Errorf("pack %s: commit must be a full 40-character commit, got %q", s.Name, s.Commit)
	}
	return nil
}

// pinnedRev checks that commit exists and, when the ref resolves, that it is
// reachable from the ref: a pin is always a commit its publisher shipped.
func pinnedRev(repo, commit, ref string) ([]byte, error) {
	rev, err := git(repo, "rev-parse", "--verify", "--end-of-options", commit+"^{commit}")
	if err != nil {
		return nil, fmt.Errorf("pinned commit %s is not in the source (run hev skills sync to fetch it): %w", commit, err)
	}
	if ref != "HEAD" {
		if _, err := git(repo, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}"); err != nil {
			return nil, fmt.Errorf("ref %s: %w", ref, err)
		}
		if _, err := git(repo, "merge-base", "--is-ancestor", commit, ref); err != nil {
			return nil, fmt.Errorf("pinned commit %s is not on %s", commit, ref)
		}
	}
	return rev, nil
}

func hasCommit(repo, commit string) bool {
	_, err := git(repo, "cat-file", "-e", commit+"^{commit}")
	return err == nil
}

// licenseFile reports whether a file name is a licence file.
func licenseFile(name string) bool {
	n := strings.ToUpper(name)
	for _, p := range []string{"LICENSE", "LICENCE", "COPYING", "UNLICENSE"} {
		if n == p || strings.HasPrefix(n, p+".") || strings.HasPrefix(n, p+"-") {
			return true
		}
	}
	return false
}

// shelfPaths is what a pack installs: its subdirectory plus every licence
// file in the directories from the repository root down to it.
func shelfPaths(repo, commit, subdir string) ([]string, error) {
	sub := strings.Trim(filepath.ToSlash(filepath.Clean(subdir)), "/")
	if sub == "." {
		sub = ""
	}
	dirs := []string{""}
	if sub != "" {
		parts := strings.Split(sub, "/")
		for i := range parts[:len(parts)-1] {
			dirs = append(dirs, strings.Join(parts[:i+1], "/"))
		}
	}
	var out []string
	for _, d := range dirs {
		spec := commit
		if d != "" {
			spec += ":" + d
		}
		b, err := git(repo, "ls-tree", "--name-only", spec)
		if err != nil {
			return nil, fmt.Errorf("subdir %q not found at %s", subdir, commit)
		}
		for _, n := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if n != "" && licenseFile(n) {
				out = append(out, path.Join(d, n))
			}
		}
	}
	if sub == "" {
		return []string{"."}, nil
	}
	if _, err := git(repo, "rev-parse", "--verify", "--end-of-options", commit+":"+sub); err != nil {
		return nil, fmt.Errorf("subdir %q not found at %s", subdir, commit)
	}
	return append(out, sub), nil
}

// describe lists a pack's skills and licences from its extracted tree.
func describe(tree string, s Source) (Pack, error) {
	p := Pack{Name: s.Name, Kind: s.Kind, URL: s.URL, Path: s.Path, Ref: s.Ref, Subdir: s.Subdir}
	p.Author, p.Home = Attribution(s.URL)
	if p.Home == "" {
		p.Home = s.Path
	}
	root := filepath.Join(tree, s.Subdir)
	seen := map[string]string{}
	err := filepath.WalkDir(root, func(dir string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, dir)
		if dir != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if strings.Count(filepath.ToSlash(rel), "/") > 4 {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			return nil
		}
		name := d.Name()
		if dir == root {
			name = filepath.Base(filepath.Clean(filepath.Join(tree, s.Subdir)))
		}
		if !skillName.MatchString(name) {
			return fmt.Errorf("skill directory %q is not a usable skill name", name)
		}
		rel, _ = filepath.Rel(tree, dir)
		if prev, dup := seen[name]; dup {
			return fmt.Errorf("two skills named %s (%s and %s); narrow the pack's subdir", name, prev, filepath.ToSlash(rel))
		}
		seen[name] = filepath.ToSlash(rel)
		p.Skills = append(p.Skills, Skill{Name: name, Path: filepath.ToSlash(rel), License: nearestLicense(tree, dir)})
		return filepath.SkipDir
	})
	if err != nil {
		return p, err
	}
	if len(p.Skills) == 0 {
		return p, fmt.Errorf("no SKILL.md under %q", s.Subdir)
	}
	sort.Slice(p.Skills, func(i, j int) bool { return p.Skills[i].Name < p.Skills[j].Name })
	p.License = nearestLicense(tree, root)
	if p.License == NoAssertion {
		ids := map[string]bool{}
		for _, sk := range p.Skills {
			ids[sk.License] = true
		}
		if len(ids) == 1 {
			p.License = p.Skills[0].License
		} else {
			p.License = "mixed"
		}
	}
	return p, nil
}

// nearestLicense detects the licence of the closest licence file in dir or
// its ancestors up to tree.
func nearestLicense(tree, dir string) string {
	for {
		entries, _ := os.ReadDir(dir)
		var names []string
		for _, e := range entries {
			if !e.IsDir() && licenseFile(e.Name()) {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			if b, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
				if id := DetectLicense(string(b)); id != NoAssertion {
					return id
				}
			}
		}
		if len(names) > 0 {
			return NoAssertion
		}
		if samePathLexical(dir, tree) {
			return NoAssertion
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return NoAssertion
		}
		dir = parent
	}
}

func samePathLexical(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }

// NoAssertion is the SPDX value for a licence that could not be determined.
const NoAssertion = "NOASSERTION"

var spdxLine = regexp.MustCompile(`(?m)SPDX-License-Identifier:\s*([A-Za-z0-9.+-]+)`)

// DetectLicense names a licence text by its SPDX id from the phrases each
// licence's text always contains. Unknown text is NOASSERTION.
func DetectLicense(text string) string {
	if m := spdxLine.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	t := strings.Join(strings.Fields(strings.ToLower(text)), " ")
	has := func(s ...string) bool {
		for _, x := range s {
			if !strings.Contains(t, x) {
				return false
			}
		}
		return true
	}
	switch {
	case has("apache license", "version 2.0"):
		return "Apache-2.0"
	case has("gnu affero general public license", "version 3"):
		return "AGPL-3.0"
	case has("gnu lesser general public license", "version 3"):
		return "LGPL-3.0"
	case has("gnu lesser general public license", "version 2.1"):
		return "LGPL-2.1"
	case has("gnu general public license", "version 3"):
		return "GPL-3.0"
	case has("gnu general public license", "version 2"):
		return "GPL-2.0"
	case has("mozilla public license version 2.0"), has("mozilla public license, version 2.0"):
		return "MPL-2.0"
	case has("this is free and unencumbered software released into the public domain"):
		return "Unlicense"
	case has("cc0 1.0 universal"):
		return "CC0-1.0"
	case has("attribution 4.0 international"):
		return "CC-BY-4.0"
	case has("permission is hereby granted, free of charge", "the above copyright notice and this permission notice shall be included"):
		return "MIT"
	case has("permission to use, copy, modify, and/or distribute this software for any purpose with or without fee"):
		return "ISC"
	case has("redistribution and use in source and binary forms", "neither the name"):
		return "BSD-3-Clause"
	case has("redistribution and use in source and binary forms"):
		return "BSD-2-Clause"
	}
	return NoAssertion
}

// Attribution is the publisher and homepage of a GitHub source URL.
func Attribution(url string) (author, home string) {
	spec, err := ParseShorthand(url)
	if err != nil || spec.Owner == "" {
		return "", ""
	}
	return spec.Owner, "https://github.com/" + spec.Owner + "/" + spec.Repo
}

// Spec is a parsed `hev skills add` argument.
type Spec struct {
	Owner, Repo string
	// URL is the clone URL written into the source entry.
	URL string
	// Subdir is the pack directory inside the repository; "" is the root.
	Subdir string
	// Ref is what followed @, or the branch of a /tree/ URL; "" is the
	// remote's default branch.
	Ref string
}

var (
	ownerRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
	repoName  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// ParseShorthand accepts the GitHub forms `npx skills add` does:
//
//	owner/repo[//subdir][@ref]       owner/repo/subdir (the //-less form)
//	github:owner/repo                github.com/owner/repo
//	https://github.com/owner/repo[.git]
//	https://github.com/owner/repo/tree/REF/subdir
//	git@github.com:owner/repo.git
func ParseShorthand(arg string) (Spec, error) {
	var s Spec
	in := strings.TrimSpace(arg)
	bad := func(why string) (Spec, error) {
		return Spec{}, fmt.Errorf("%q is not a GitHub pack (%s); use OWNER/REPO[//SUBDIR][@REF]", arg, why)
	}
	if in == "" {
		return bad("empty")
	}
	switch {
	case strings.HasPrefix(in, "git@github.com:"):
		in = strings.TrimPrefix(in, "git@github.com:")
	case strings.HasPrefix(in, "github:"):
		in = strings.TrimPrefix(in, "github:")
	default:
		for _, p := range []string{"https://", "http://", "ssh://git@", "git://"} {
			in = strings.TrimPrefix(in, p)
		}
		in = strings.TrimPrefix(in, "www.")
		if r, ok := strings.CutPrefix(in, "github.com/"); ok {
			in = r
		} else if strings.Contains(strings.SplitN(in, "/", 2)[0], ".") || strings.Contains(in, "://") {
			return bad("only github.com repositories")
		}
	}
	// @ref is the last @ outside the owner, so a ref may hold slashes.
	if i := strings.LastIndex(in, "@"); i > 0 {
		s.Ref, in = in[i+1:], in[:i]
		if s.Ref == "" || strings.HasPrefix(s.Ref, "-") || strings.ContainsAny(s.Ref, " \t:~^?*[\\") || strings.Contains(s.Ref, "..") {
			return bad("invalid ref")
		}
	}
	head, sub, hasSub := strings.Cut(in, "//")
	parts := strings.Split(strings.Trim(head, "/"), "/")
	if len(parts) < 2 {
		return bad("missing repository")
	}
	s.Owner, s.Repo = parts[0], strings.TrimSuffix(parts[1], ".git")
	rest := parts[2:]
	if len(rest) >= 2 && rest[0] == "tree" && !hasSub {
		if s.Ref != "" {
			return bad("both a /tree/ ref and @ref")
		}
		// /tree/REF/path: the ref is one segment, as GitHub's own links
		// are for ordinary branch and tag names.
		s.Ref = rest[1]
		rest = rest[2:]
	} else if len(rest) > 0 && hasSub {
		return bad("a path both before and after //")
	}
	if hasSub {
		s.Subdir = strings.Trim(sub, "/")
	} else {
		s.Subdir = strings.Join(rest, "/")
	}
	if !ownerRepo.MatchString(s.Owner) || !repoName.MatchString(s.Repo) || s.Repo == "." || s.Repo == ".." {
		return bad("invalid owner or repository")
	}
	if s.Subdir != "" && (!cleanSub(s.Subdir) || path.Clean(s.Subdir) != s.Subdir) {
		return bad("invalid subdir")
	}
	s.URL = "https://github.com/" + s.Owner + "/" + s.Repo + ".git"
	return s, nil
}

// DefaultName is a pack's short name: the last subdir segment that is not a
// generic word, else the repository, else the owner.
func (s Spec) DefaultName() string {
	generic := map[string]bool{"skills": true, "plugins": true, "packages": true, "agent-skills": true, "src": true}
	segs := strings.Split(s.Subdir, "/")
	for i := len(segs) - 1; i >= 0; i-- {
		if n := strings.ToLower(segs[i]); n != "" && !generic[n] && packName.MatchString(n) {
			return n
		}
	}
	if n := strings.ToLower(s.Repo); !generic[n] && packName.MatchString(n) {
		return n
	}
	return strings.ToLower(s.Owner)
}

// Packs lists the installed shelf packs from the ownership manifest.
func Packs(home string) (map[string]Pack, error) {
	b, err := os.ReadFile(filepath.Join(home, ".local", "state", "hev-skills", "sources.json"))
	if os.IsNotExist(err) {
		return map[string]Pack{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Shelf == nil {
		m.Shelf = map[string]Pack{}
	}
	return m.Shelf, nil
}

// Enabled is one shelf skill a run enables.
type Enabled struct {
	Pack    string `json:"pack"`
	Skill   string `json:"skill"`
	Kind    string `json:"kind"`
	Commit  string `json:"commit"`
	License string `json:"license"`
	// Dir is the skill's directory in the pinned store.
	Dir string `json:"dir"`
}

// Name is the pack/skill form orders use.
func (e Enabled) Name() string { return e.Pack + "/" + e.Skill }

// ValidSkillRef checks the pack/skill form without reading any config.
func ValidSkillRef(ref string) error {
	pack, skill, ok := strings.Cut(ref, "/")
	if !ok || !packName.MatchString(pack) || !skillName.MatchString(skill) {
		return fmt.Errorf("skill %q: name a shelf skill as pack/skill", ref)
	}
	return nil
}

// Enable resolves a run's pack/skill names against the configured shelf and
// the installed pins. The configured commit must be the installed one, every
// skill must exist, two packs may not give a skill the same name, and at most
// one mode pack may be enabled.
func Enable(home string, c Config, refs []string) ([]Enabled, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	packs, err := Packs(home)
	if err != nil {
		return nil, err
	}
	configured := map[string]Source{}
	for _, s := range c.Sources {
		if s.Shelf() {
			configured[s.Name] = s
		}
	}
	var out []Enabled
	seen := map[string]string{}
	modes := map[string]bool{}
	for _, ref := range refs {
		if err := ValidSkillRef(ref); err != nil {
			return nil, err
		}
		packName, skill, _ := strings.Cut(ref, "/")
		src, ok := configured[packName]
		if !ok {
			return nil, fmt.Errorf("skill %s: no shelf pack %q is configured on this host (hev skills shelf lists them)", ref, packName)
		}
		p, ok := packs[packName]
		if !ok || p.Commit != src.Commit {
			return nil, fmt.Errorf("skill %s: pack %s is pinned to %s but not installed at that commit; run hev skills sync", ref, packName, short(src.Commit))
		}
		var found *Skill
		for i := range p.Skills {
			if p.Skills[i].Name == skill {
				found = &p.Skills[i]
			}
		}
		if found == nil {
			return nil, fmt.Errorf("skill %s: pack %s has no skill %q at %s", ref, packName, skill, short(p.Commit))
		}
		if prev, dup := seen[skill]; dup {
			if prev == ref {
				return nil, fmt.Errorf("skill %s is enabled twice", ref)
			}
			return nil, fmt.Errorf("skills %s and %s share the name %s; a harness can load only one", prev, ref, skill)
		}
		seen[skill] = ref
		dir := filepath.Join(p.Pin, filepath.FromSlash(found.Path))
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			return nil, fmt.Errorf("skill %s: pinned copy missing at %s; run hev skills sync", ref, dir)
		}
		// Kind is the operator's classification in config, not install data.
		if src.Kind == KindMode {
			modes[packName] = true
		}
		out = append(out, Enabled{Pack: packName, Skill: skill, Kind: src.Kind, Commit: p.Commit, License: found.License, Dir: dir})
	}
	if len(modes) > 1 {
		var names []string
		for n := range modes {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("this run enables %d mode packs (%s); a mode owns the workflow, so a run takes at most one", len(names), strings.Join(names, ", "))
	}
	return out, nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// pinPattern matches a path inside a pinned source release.
var pinPattern = regexp.MustCompile(`[/\\]\.local[/\\]opt[/\\]hev-skills-[0-9a-f]{16}-([0-9a-f]{40})([/\\]|$)`)

// PinnedCommit is the commit of the pinned store release a path is in, or "".
func PinnedCommit(p string) string {
	if m := pinPattern.FindStringSubmatch(p); m != nil {
		return m[1]
	}
	return ""
}
