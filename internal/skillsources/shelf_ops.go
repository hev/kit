package skillsources

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// sourceID is the stable identity Run gives a source.
func sourceID(s Source) string {
	return hash([]byte(s.URL + "\x00" + s.Path + "\x00" + s.Ref + "\x00" + s.Subdir))[:16]
}

// ResolveRemote turns an optional ref from `hev skills add` into the full ref
// a source entry records and the commit to pin. An empty ref is the remote's
// default branch; a full commit pins that commit on the default branch.
func ResolveRemote(url, ref string) (fullRef, commit string, err error) {
	lsRemote := func(args ...string) (string, error) {
		c := exec.Command("git", append([]string{"ls-remote"}, args...)...)
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		b, err := c.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return "", fmt.Errorf("git ls-remote %s: %w: %s", url, err, ee.Stderr)
			}
			return "", fmt.Errorf("git ls-remote %s: %w", url, err)
		}
		return string(b), nil
	}
	pinned := ""
	if fullSHA.MatchString(ref) {
		pinned, ref = ref, ""
	}
	if ref == "" {
		out, err := lsRemote("--symref", "--", url, "HEAD")
		if err != nil {
			return "", "", err
		}
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) == 3 && f[0] == "ref:" && f[2] == "HEAD" {
				fullRef = f[1]
			} else if len(f) == 2 && f[1] == "HEAD" {
				commit = f[0]
			}
		}
		if fullRef == "" || commit == "" {
			return "", "", fmt.Errorf("%s has no default branch", url)
		}
	} else {
		candidates := []string{ref}
		if !strings.HasPrefix(ref, "refs/") {
			candidates = []string{"refs/heads/" + ref, "refs/tags/" + ref}
		}
		out, err := lsRemote(append([]string{"--", url}, candidates...)...)
		if err != nil {
			return "", "", err
		}
		found := map[string]string{}
		for _, line := range strings.Split(out, "\n") {
			if f := strings.Fields(line); len(f) == 2 {
				found[f[1]] = f[0]
			}
		}
		for _, c := range candidates {
			// An annotated tag's peeled commit is listed as TAG^{}.
			if sha, ok := found[c+"^{}"]; ok {
				fullRef, commit = c, sha
				break
			}
			if sha, ok := found[c]; ok {
				fullRef, commit = c, sha
				break
			}
		}
		if fullRef == "" {
			return "", "", fmt.Errorf("%s has no branch or tag %q", url, ref)
		}
	}
	if pinned != "" {
		commit = pinned
	}
	return fullRef, commit, nil
}

// AddSourceText appends a shelf pack entry to config text, keeping the rest
// of the file byte for byte.
func AddSourceText(text string, s Source) (string, error) {
	if err := s.validateShelf(); err != nil {
		return "", err
	}
	if s.URL == "" || s.Path != "" {
		return "", fmt.Errorf("a shelf pack added by hev skills add needs a url")
	}
	var c Config
	if err := decodeText(text, &c); err != nil {
		return "", err
	}
	for _, o := range c.Sources {
		if o.Name == s.Name {
			return "", fmt.Errorf("a pack named %s is already configured; pass --name", s.Name)
		}
		if o.URL == s.URL && o.Ref == s.Ref && o.Subdir == s.Subdir {
			return "", fmt.Errorf("%s//%s is already configured as %q", s.URL, s.Subdir, o.Name)
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	q := strconv.Quote
	text += "[[skills.sources]]\n"
	text += "name = " + q(s.Name) + "\n"
	text += "kind = " + q(s.Kind) + "\n"
	text += "url = " + q(s.URL) + "\n"
	text += "ref = " + q(s.Ref) + "\n"
	if s.Subdir != "" {
		text += "subdir = " + q(s.Subdir) + "\n"
	}
	text += "commit = " + q(s.Commit) + "\n"
	return text, nil
}

var (
	tableHeader = regexp.MustCompile(`^\s*\[`)
	nameLine    = regexp.MustCompile(`^\s*name\s*=\s*"([^"]*)"\s*(#.*)?$`)
	commitLine  = regexp.MustCompile(`^(\s*commit\s*=\s*)"[^"]*"(\s*(#.*)?)$`)
)

// SetCommitText moves one pack's pin in config text. Only the commit line of
// that pack's [[skills.sources]] block changes.
func SetCommitText(text, pack, commit string) (string, error) {
	if !fullSHA.MatchString(commit) {
		return "", fmt.Errorf("a pin is a full 40-character commit, got %q", commit)
	}
	lines := strings.SplitAfter(text, "\n")
	start := -1
	for i := 0; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if l == "[[skills.sources]]" {
			start = i
			continue
		}
		if start < 0 {
			continue
		}
		if tableHeader.MatchString(lines[i]) {
			start = -1
			continue
		}
		if m := nameLine.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n")); m != nil && m[1] == pack {
			for j := start + 1; j < len(lines) && (j == start+1 || !tableHeader.MatchString(lines[j])); j++ {
				body := strings.TrimRight(lines[j], "\r\n")
				if commitLine.MatchString(body) {
					lines[j] = commitLine.ReplaceAllString(body, `${1}"`+commit+`"${2}`) + lines[j][len(body):]
					return strings.Join(lines, ""), nil
				}
			}
			return "", fmt.Errorf("pack %s has no commit line to move", pack)
		}
	}
	return "", fmt.Errorf("no shelf pack %s in the config", pack)
}

func decodeText(text string, c *Config) error {
	f, err := os.CreateTemp("", "hev-skills-config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	*c, err = Load(f.Name())
	return err
}

// FindPack returns the configured shelf source with this name.
func FindPack(c Config, name string) (Source, error) {
	for _, s := range c.Sources {
		if s.Shelf() && s.Name == name {
			return s, nil
		}
	}
	return Source{}, fmt.Errorf("no shelf pack %q is configured (hev skills shelf lists them)", name)
}

// mirror is the local Git repository holding a pack's history, fetched when
// fetch is set. Path sources are read where they are.
func mirror(home string, s Source, fetch bool) (string, error) {
	if s.Path != "" {
		return filepath.EvalSymlinks(s.Path)
	}
	repo := filepath.Join(home, ".local", "state", "hev-skills", "repos", sourceID(s))
	if _, err := os.Stat(repo); os.IsNotExist(err) {
		return "", fmt.Errorf("pack %s is not synced yet; run hev skills sync", s.Name)
	}
	if fetch {
		if _, err := git(repo, "fetch", "--prune", "origin"); err != nil {
			return "", err
		}
	}
	return repo, nil
}

// Diff shows what changed upstream in a pack since its pin: the commits on its
// ref that touch the pack, a stat, and (with patch) the full diff. It moves
// nothing. to, when set, compares against that commit instead of the ref.
func Diff(home string, c Config, name, to string, patch bool, out io.Writer) (tip string, err error) {
	s, err := FindPack(c, name)
	if err != nil {
		return "", err
	}
	repo, err := mirror(home, s, true)
	if err != nil {
		return "", err
	}
	ref := s.Ref
	if ref == "" {
		ref = "HEAD"
	}
	target := ref
	if to != "" {
		target = to
	}
	b, err := git(repo, "rev-parse", "--verify", "--end-of-options", target+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s: %w", target, err)
	}
	tip = strings.TrimSpace(string(b))
	if tip == s.Commit {
		fmt.Fprintf(out, "pack %s: pinned %s is the tip of %s; nothing to review\n", name, short(s.Commit), target)
		return tip, nil
	}
	spec := map[string]bool{}
	for _, c := range []string{s.Commit, tip} {
		paths, err := shelfPaths(repo, c, s.Subdir)
		if err != nil {
			continue
		}
		for _, p := range paths {
			spec[p] = true
		}
	}
	pathspec := []string{"--"}
	for p := range spec {
		pathspec = append(pathspec, p)
	}
	fmt.Fprintf(out, "pack %s (%s): pinned %s, upstream %s at %s\n\n", name, s.Kind, short(s.Commit), target, short(tip))
	log, err := git(repo, append([]string{"log", "--no-color", "--format=%h %ad %an  %s", "--date=short", s.Commit + ".." + tip}, pathspec...)...)
	if err != nil {
		return "", err
	}
	if len(strings.TrimSpace(string(log))) == 0 {
		fmt.Fprintln(out, "no upstream commit touches this pack since the pin")
	} else {
		fmt.Fprint(out, string(log))
	}
	stat, err := git(repo, append([]string{"diff", "--no-color", "--stat", s.Commit, tip}, pathspec...)...)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(out, "\n%s", stat)
	if patch {
		d, err := git(repo, append([]string{"diff", "--no-color", s.Commit, tip}, pathspec...)...)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(out, "\n%s", d)
	}
	return tip, nil
}

// ResolveBump checks a bump target: a commit (not a ref name) present in the
// pack's history and reachable from its ref. It returns the full commit.
func ResolveBump(home string, c Config, name, to string) (Source, string, error) {
	s, err := FindPack(c, name)
	if err != nil {
		return s, "", err
	}
	if !regexp.MustCompile(`^[0-9a-f]{7,40}$`).MatchString(to) {
		return s, "", fmt.Errorf("bump moves a pin only to a reviewed commit; --to must be a commit SHA, got %q", to)
	}
	repo, err := mirror(home, s, true)
	if err != nil {
		return s, "", err
	}
	b, err := git(repo, "rev-parse", "--verify", "--end-of-options", to+"^{commit}")
	if err != nil {
		return s, "", fmt.Errorf("commit %s is not in pack %s's history", to, name)
	}
	full := strings.TrimSpace(string(b))
	if !strings.HasPrefix(full, to) {
		return s, "", fmt.Errorf("%s is not a commit SHA", to)
	}
	ref := s.Ref
	if ref == "" {
		ref = "HEAD"
	}
	if _, err := pinnedRev(repo, full, ref); err != nil {
		return s, "", err
	}
	return s, full, nil
}
