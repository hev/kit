package index

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/trace"
)

// startContextMaxRead bounds how much of one instruction or skill file is
// read to size and hash it.
const startContextMaxRead = 1 << 20

// sessionStartContext completes a session row's starting-context record.
// What the transcript recorded is kept; what it did not is derived from the
// working directory and home on this host now, at capture, and marked
// "derived" (today's files, not necessarily the session's). Paths are made
// relative to the repository or ~, the dispatch context digest is copied
// in, and only sizes and hashes are kept.
func sessionStartContext(row *trace.SessionRow, home string, scrub func(string) string) {
	c := trace.StartContext{}
	if row.StartContext != nil {
		c = *row.StartContext
		c.Instructions = append([]trace.InstructionRef(nil), c.Instructions...)
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	repo := repoRoot(row.Workdir)
	if len(c.Instructions) == 0 {
		for _, p := range instructionCandidates(row.Harness, row.Workdir, repo, home) {
			if b, ok := readBounded(p); ok {
				c.AddInstruction(p, "", "derived", b)
			}
		}
	}
	if c.SkillsSource == "" {
		n, size := 0, int64(0)
		for _, root := range skillRoots(row.Harness, repo, home) {
			for _, s := range skillDescriptions(root) {
				n++
				size += s
			}
		}
		if n > 0 {
			c.SetSkills(n, size, "derived")
		}
	}
	for i := range c.Instructions {
		p := relativePath(c.Instructions[i].Path, repo, home)
		if scrub != nil {
			p = scrub(p)
		}
		c.Instructions[i].Path = p
	}
	c.ContextDigest = row.ContextDigest
	if c.Source == "" {
		c.Source = "none"
	}
	row.StartContext = &c
	row.StartContextBytes = c.Bytes()
}

// repoRoot is the nearest ancestor of dir holding .git, or "".
func repoRoot(dir string) string {
	if dir == "" || !filepath.IsAbs(dir) {
		return ""
	}
	for p := filepath.Clean(dir); ; p = filepath.Dir(p) {
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p
		}
		if filepath.Dir(p) == p {
			return ""
		}
	}
}

func relativePath(p, repo, home string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	if repo != "" {
		if rel, err := filepath.Rel(repo, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	if home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return p
}

// instructionCandidates lists, in load order, the instruction files a
// harness reads at start for a working directory: its home file, then each
// directory's file from the repository root (or the working directory
// itself outside a repository) down to the working directory.
func instructionCandidates(name, workdir, repo, home string) []string {
	names := []string{"AGENTS.md"}
	out := []string{}
	if d, ok := harness.Lookup(name); ok && name == d.TraceName && d.Skills != nil {
		names = d.Skills.ProjectInstructions
		out = append(out, filepath.Join(home, filepath.FromSlash(d.Skills.UserInstructions)))
	}
	if workdir == "" || !filepath.IsAbs(workdir) {
		return out
	}
	top := repo
	if top == "" {
		top = workdir
	}
	var dirs []string
	for p := filepath.Clean(workdir); ; p = filepath.Dir(p) {
		dirs = append(dirs, p)
		if p == top || filepath.Dir(p) == p {
			break
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		for _, n := range names {
			out = append(out, filepath.Join(dirs[i], n))
		}
	}
	return out
}

// skillRoots lists the directories a harness offers skills from.
func skillRoots(name, repo, home string) []string {
	var roots []string
	d, ok := harness.Lookup(name)
	if !ok || name != d.TraceName || d.Skills == nil {
		return roots
	}
	if d.Skills.UserExtensions != "" {
		paths, _ := filepath.Glob(filepath.Join(home, filepath.FromSlash(d.Skills.UserExtensions), "*", "skills"))
		roots = append(roots, paths...)
	}
	for _, path := range d.Skills.User {
		roots = append(roots, filepath.Join(home, filepath.FromSlash(path)))
	}
	if repo != "" {
		for _, path := range d.Skills.Project {
			roots = append(roots, filepath.Join(repo, filepath.FromSlash(path)))
		}
	}
	return roots
}

// SkillDescription is one installed skill: its name, directory and the
// size of the name and description a harness lists for it.
type SkillDescription struct {
	Name  string
	Dir   string
	Bytes int64
}

// InstalledSkills lists the skills under roots (each root/NAME/SKILL.md,
// and one level of nesting such as Codex's .system), by name.
func InstalledSkills(roots []string) []SkillDescription {
	var out []SkillDescription
	seen := map[string]bool{}
	for _, root := range roots {
		paths, _ := filepath.Glob(filepath.Join(root, "*", "SKILL.md"))
		nested, _ := filepath.Glob(filepath.Join(root, "*", "*", "SKILL.md"))
		for _, p := range append(paths, nested...) {
			b, ok := readBounded(p)
			if !ok {
				continue
			}
			name, desc := frontmatter(b)
			if name == "" {
				name = filepath.Base(filepath.Dir(p))
			}
			name = trace.NormalizeSkillName(name)
			if name == "" || seen[root+"\x00"+name] {
				continue
			}
			seen[root+"\x00"+name] = true
			out = append(out, SkillDescription{Name: name, Dir: filepath.Dir(p), Bytes: int64(len(name) + len(desc))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func skillDescriptions(root string) []int64 {
	var out []int64
	for _, s := range InstalledSkills([]string{root}) {
		out = append(out, s.Bytes)
	}
	return out
}

// HarnessSkillRoots is every skill root this host's harnesses read, for
// hev traces stats --installed.
func HarnessSkillRoots(home string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range []string{"claude_code", "codex", "pi", harness.Gemini} {
		for _, r := range skillRoots(h, "", home) {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	return out
}

// frontmatter reads a SKILL.md's name and description.
func frontmatter(b []byte) (name, desc string) {
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", ""
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return "", ""
	}
	lines := strings.Split(text[4:4+end], "\n")
	for i := 0; i < len(lines); i++ {
		key, value, ok := strings.Cut(lines[i], ":")
		if !ok || strings.HasPrefix(key, " ") {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "|" || value == ">" || value == "|-" || value == ">-" || value == "" {
			var parts []string
			for i+1 < len(lines) && (strings.HasPrefix(lines[i+1], " ") || strings.TrimSpace(lines[i+1]) == "") {
				i++
				parts = append(parts, strings.TrimSpace(lines[i]))
			}
			value = strings.TrimSpace(strings.Join(parts, " "))
		}
		value = strings.Trim(value, `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			name = value
		case "description":
			desc = value
		}
	}
	return name, desc
}

func readBounded(p string) ([]byte, bool) {
	f, err := os.Open(p)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(f, startContextMaxRead))
	return b, err == nil
}
