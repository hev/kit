// Package skillsources installs explicitly configured repositories as pinned skills.
package skillsources

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Source struct {
	URL    string `toml:"url"`
	Path   string `toml:"path"`
	Ref    string `toml:"ref"`
	Subdir string `toml:"subdir"`
}
type Config struct {
	Machine string   `toml:"machine"`
	Role    string   `toml:"role"`
	Sources []Source `toml:"sources"`
}

func Load(path string) (Config, error) {
	var c struct {
		Skills Config `toml:"skills"`
	}
	_, e := toml.DecodeFile(path, &c)
	if os.IsNotExist(e) {
		e = nil
	}
	for i := range c.Skills.Sources {
		s := &c.Skills.Sources[i]
		if s.Path != "" && !filepath.IsAbs(s.Path) {
			s.Path = filepath.Join(filepath.Dir(path), s.Path)
		}
	}
	return c.Skills, e
}

type entry struct {
	Target string
	Digest string
	Source string
}
type manifest struct {
	Commits map[string]string
	Links   map[string]entry
}

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func git(dir string, args ...string) ([]byte, error) {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	b, e := c.Output()
	if e != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], e, stderr.String())
	}
	return b, nil
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}
func cleanSub(s string) bool {
	return !strings.HasPrefix(s, "/") && !filepath.IsAbs(s) && filepath.Clean(s) != ".." && !strings.HasPrefix(filepath.Clean(s), ".."+string(filepath.Separator)) && !strings.ContainsAny(s, "\\:")
}

// Installed lists published destinations and their pinned targets from the
// ownership manifest.
func Installed(home string) (map[string]string, error) {
	links := map[string]string{}
	b, e := os.ReadFile(filepath.Join(home, ".local", "state", "hev-skills", "sources.json"))
	if os.IsNotExist(e) {
		return links, nil
	}
	if e != nil {
		return nil, e
	}
	var m manifest
	if e = json.Unmarshal(b, &m); e != nil {
		return nil, e
	}
	for dest, l := range m.Links {
		links[dest] = l.Target
	}
	return links, nil
}

// Run resolves each source, reports drift, and optionally publishes a new install.
// A process lock prevents overlapping scheduled invocations. Checks do not fetch.
func Run(home string, c Config, sync bool, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	var err error
	home, err = filepath.Abs(home)
	if err != nil {
		return err
	}
	root := filepath.Join(home, ".local", "state", "hev-skills")
	if len(c.Sources) == 0 {
		if _, e := os.Stat(filepath.Join(root, "sources.json")); os.IsNotExist(e) {
			return nil
		}
	}
	if e := safeParents(home, filepath.Join(root, "lock")); e != nil {
		return e
	}
	if e := safeParents(home, filepath.Join(home, ".local", "opt", "placeholder")); e != nil {
		return e
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	lock := filepath.Join(root, "lock")
	if e := os.Mkdir(lock, 0700); e != nil {
		return fmt.Errorf("skills sync already running (or stale lock at %s): %w", lock, e)
	}
	defer os.Remove(lock)
	manifestPath := filepath.Join(root, "sources.json")
	var previous manifest
	if b, err := os.ReadFile(manifestPath); err == nil {
		if err = json.Unmarshal(b, &previous); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	old := previous.Links
	if c.Role != "" && c.Role != "sender" && c.Role != "host" {
		return fmt.Errorf("skills role must be sender or host")
	}
	desired := map[string]entry{}
	commits := map[string]string{}
	drift := false
	sourceDrift := map[string]bool{}
	stage, e := os.MkdirTemp(root, "prepare-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	for i, s := range c.Sources {
		if (s.Path == "") == (s.URL == "") || !cleanSub(s.Subdir) {
			return fmt.Errorf("source %d: set exactly one url/path and a relative subdir", i+1)
		}
		id := hash([]byte(s.URL + "\x00" + s.Path + "\x00" + s.Ref + "\x00" + s.Subdir))[:16]
		repo := s.Path
		if repo != "" {
			var err error
			repo, err = filepath.EvalSymlinks(repo)
			if err != nil {
				return err
			}
		}
		remote := s.URL != ""
		if remote {
			repo = filepath.Join(root, "repos", id)
			if _, e = os.Stat(repo); os.IsNotExist(e) {
				if !sync {
					fmt.Fprintf(out, "source %s: missing\n", id)
					drift = true
					continue
				}
				if e = os.MkdirAll(filepath.Dir(repo), 0700); e != nil {
					return e
				}
				cmd := exec.Command("git", "clone", "--mirror", "--", s.URL, repo)
				if b, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("clone source: %w: %s", err, b)
				}
			} else if sync {
				if _, e = git(repo, "fetch", "--prune", "origin"); e != nil {
					return e
				}
			}
		}
		ref := s.Ref
		if ref == "" {
			ref = "HEAD"
		}
		if strings.HasPrefix(ref, "-") {
			return fmt.Errorf("invalid source ref")
		}
		// Only a repository root is a Git source; nested directories are snapshots.
		top, topErr := git(repo, "rev-parse", "--show-toplevel")
		bare, _ := git(repo, "rev-parse", "--is-bare-repository")
		isGit := remote || strings.TrimSpace(string(bare)) == "true" || (topErr == nil && samePath(strings.TrimSpace(string(top)), repo))
		var rev []byte
		var err error
		if isGit {
			rev, err = git(repo, "rev-parse", "--verify", ref+"^{commit}")
		} else {
			err = fmt.Errorf("not a repository root")
		}
		commit := ""
		tree := filepath.Join(stage, id)
		if err == nil {
			commit = strings.TrimSpace(string(rev))
			prev := previous.Commits[id]
			if prev != "" && prev != commit {
				if _, e = git(repo, "merge-base", "--is-ancestor", prev, commit); e != nil {
					return fmt.Errorf("source %s refuses non-fast-forward update", id)
				}
			}
			b, err := git(repo, "-c", "core.autocrlf=false", "-c", "core.eol=lf", "archive", "--format=tar", commit)
			if err != nil {
				return err
			}
			if err = extract(b, tree); err != nil {
				return err
			}
		} else {
			_, gitErr := git(repo, "rev-parse", "--git-dir")
			if remote || s.Ref != "" || (isGit && gitErr == nil) {
				return err
			}
			if err = copyTree(repo, tree); err != nil {
				return err
			}
			commit, e = digest(tree)
			if e != nil {
				return e
			}
		}
		selected := filepath.Join(tree, s.Subdir)
		pins := filepath.Join(home, ".local", "opt", "hev-skills-"+id+"-"+commit)
		var links map[string]string
		_, statErr := os.Stat(selected)
		if os.IsNotExist(statErr) && previous.Commits[id] != "" {
			links = map[string]string{}
		} else {
			links, err = plan(home, selected, filepath.Join(pins, s.Subdir), c)
		}
		if err != nil {
			return fmt.Errorf("source %s: %w", id, err)
		}
		if sync {
			if _, err = os.Stat(pins); os.IsNotExist(err) {
				if err = os.MkdirAll(filepath.Dir(pins), 0755); err != nil {
					return err
				}
				if err = os.Rename(tree, pins); err != nil {
					return err
				}
			}
		}
		for dest, target := range links {
			if _, ok := desired[dest]; ok {
				return fmt.Errorf("source collision at %s", dest)
			}
			digestPath := filepath.Join(tree, strings.TrimPrefix(target, pins+string(filepath.Separator)))
			if _, e := os.Stat(pins); e == nil {
				digestPath = target
			}
			d, err := digest(digestPath)
			if err != nil {
				return err
			}
			desired[dest] = entry{Target: target, Digest: d, Source: id}
		}
		commits[id] = commit
		if previous.Commits[id] != commit {
			drift = true
			sourceDrift[id] = true
		}
	}
	for dest, want := range desired {
		if err := safeParents(home, dest); err != nil {
			return err
		}
		prev, owned := old[dest]
		_, err := os.Lstat(dest)
		if err == nil && (!owned || !matches(dest, prev)) {
			return fmt.Errorf("foreign or locally edited file at %s", dest)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if !matches(dest, want) {
			drift = true
			sourceDrift[want.Source] = true
			fmt.Fprintf(out, "%s: drift\n", dest)
		}
	}
	for dest, prev := range old {
		if err := safeParents(home, dest); err != nil {
			return err
		}
		if _, ok := desired[dest]; !ok {
			if _, err := os.Lstat(dest); err == nil && !matches(dest, prev) {
				return fmt.Errorf("foreign or locally edited file at %s", dest)
			}
			drift = true
			sourceDrift[prev.Source] = true
			fmt.Fprintf(out, "%s: retired\n", dest)
		}
	}
	ids := make([]string, 0, len(commits))
	for id := range commits {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		state := "current"
		if sourceDrift[id] {
			state = "drift"
		}
		fmt.Fprintf(out, "source %s: %s (%s)\n", id, state, commits[id])
	}
	for id := range previous.Commits {
		if _, ok := commits[id]; !ok {
			drift = true
			fmt.Fprintf(out, "source %s: retired\n", id)
		}
	}
	if !sync {
		if drift {
			return fmt.Errorf("skill sources need synchronization")
		}
		return nil
	}
	return publish(stage, old, desired, func() error {
		temp := filepath.Join(stage, "manifest.json")
		if err := writeJSON(temp, manifest{Commits: commits, Links: desired}); err != nil {
			return err
		}
		// Keep the previous manifest available for rollback on Windows too.
		backup := filepath.Join(stage, "manifest-old.json")
		hadOld := false
		if _, err := os.Stat(manifestPath); err == nil {
			if err = os.Rename(manifestPath, backup); err != nil {
				return err
			}
			hadOld = true
		}
		if err := os.Rename(temp, manifestPath); err != nil {
			if hadOld {
				if restore := os.Rename(backup, manifestPath); restore != nil {
					return fmt.Errorf("%w; restore manifest: %v", err, restore)
				}
			}
			return err
		}
		return nil
	})
}

func extract(b []byte, dest string) error {
	if e := os.MkdirAll(dest, 0755); e != nil {
		return e
	}
	tr := tar.NewReader(bytes.NewReader(b))
	for {
		h, e := tr.Next()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		if !cleanSub(h.Name) {
			return fmt.Errorf("unsafe archive path")
		}
		p := filepath.Join(dest, filepath.FromSlash(h.Name))
		switch h.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			e = os.MkdirAll(p, 0755)
		case tar.TypeReg:
			if e = os.MkdirAll(filepath.Dir(p), 0755); e == nil {
				var f *os.File
				f, e = os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_EXCL, fs.FileMode(h.Mode)&0777)
				if e == nil {
					_, e = io.Copy(f, tr)
					if e == nil {
						e = f.Chmod(fs.FileMode(h.Mode) & 0777)
					}
					ce := f.Close()
					if e == nil {
						e = ce
					}
				}
			}
		default:
			return fmt.Errorf("source symlinks and special files are unsupported: %s", h.Name)
		}
		if e != nil {
			return e
		}
	}
}
func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(src, p)
		if e != nil {
			return e
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		to := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0755)
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source special file unsupported: %s", p)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		if e := os.WriteFile(to, b, info.Mode().Perm()); e != nil {
			return e
		}
		return os.Chmod(to, info.Mode().Perm())
	})
}
func digest(path string) (string, error) {
	var b bytes.Buffer
	e := filepath.WalkDir(path, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		r, _ := filepath.Rel(path, p)
		info, e := d.Info()
		if e != nil {
			return e
		}
		fmt.Fprintf(&b, "%s:%v\n", filepath.ToSlash(r), info.Mode().Perm()&0111)
		if !d.IsDir() {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("special file %s", p)
			}
			raw, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			fmt.Fprintln(&b, hash(raw))
		}
		return nil
	})
	return hash(b.Bytes()), e
}
func matches(dest string, w entry) bool {
	if target, e := os.Readlink(dest); e == nil {
		if target != w.Target {
			return false
		}
		d, e := digest(w.Target)
		return e == nil && d == w.Digest
	}
	d, e := digest(dest)
	return e == nil && d == w.Digest
}

// publish stages every destination first and rolls back all changed paths on error.
// Windows uses copies, including command files, without symlink privileges.
func publish(stage string, old, next map[string]entry, commit func() error) (err error) {
	keys := map[string]bool{}
	for p := range old {
		keys[p] = true
	}
	for p := range next {
		keys[p] = true
	}
	paths := make([]string, 0, len(keys))
	for p := range keys {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	type change struct {
		dest, backup string
		installed    bool
	}
	var done []change
	defer func() {
		if err != nil {
			for i := len(done) - 1; i >= 0; i-- {
				c := done[i]
				if c.installed {
					_ = os.RemoveAll(c.dest)
				}
				if c.backup != "" {
					if e := os.Rename(c.backup, c.dest); e != nil {
						err = fmt.Errorf("%w; rollback: %v", err, e)
					}
				}
			}
		}
	}()
	prepared := map[string]string{}
	defer func() {
		for _, tmp := range prepared {
			_ = os.RemoveAll(tmp)
		}
	}()
	for _, dest := range paths {
		w, keep := next[dest]
		if !keep || matches(dest, w) {
			continue
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		tmp := dest + ".hev-new"
		if _, e := os.Lstat(tmp); !os.IsNotExist(e) {
			return fmt.Errorf("staging path exists: %s", tmp)
		}
		prepared[dest] = tmp
		if runtime.GOOS == "windows" {
			info, e := os.Stat(w.Target)
			if e != nil {
				return e
			}
			if info.IsDir() {
				err = copyTree(w.Target, tmp)
			} else {
				var b []byte
				b, err = os.ReadFile(w.Target)
				if err == nil {
					err = os.WriteFile(tmp, b, info.Mode().Perm())
				}
			}
		} else {
			err = os.Symlink(w.Target, tmp)
		}
		if err != nil {
			return err
		}
	}
	for i, dest := range paths {
		_, keep := next[dest]
		tmp, ready := prepared[dest]
		if keep && !ready {
			continue
		}

		c := change{dest: dest}
		if _, e := os.Lstat(dest); e == nil {
			c.backup = filepath.Join(stage, fmt.Sprintf("backup-%d", i))
			if target, e := os.Readlink(dest); e == nil && keep && runtime.GOOS != "windows" {
				// Preserve a rollback link while atomically replacing the destination.
				err = os.Symlink(target, c.backup)
			} else {
				err = os.Rename(dest, c.backup)
			}
			if err != nil {
				return err
			}
		}
		done = append(done, c)
		if keep {
			if err = os.Rename(tmp, dest); err != nil {
				_ = os.RemoveAll(tmp)
				return err
			}
			done[len(done)-1].installed = true
		}
	}
	return commit()
}

func safeParents(home, dest string) error {
	rel, e := filepath.Rel(home, dest)
	if e != nil || !cleanSub(filepath.ToSlash(rel)) {
		return fmt.Errorf("destination escapes home: %s", dest)
	}
	for p := filepath.Dir(dest); p != home; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("foreign ancestor at %s", p)
		}
	}
	return nil
}

// samePath compares canonical roots, including symlinked roots on macOS.
func samePath(a, b string) bool {
	aa, ea := filepath.EvalSymlinks(a)
	bb, eb := filepath.EvalSymlinks(b)
	return ea == nil && eb == nil && filepath.Clean(aa) == filepath.Clean(bb)
}
