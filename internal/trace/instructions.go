package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const InstructionMaxBytes = 1 << 20
const InstructionMaxFiles = 4096
const InstructionImportDepth = 8

// InstructionFile is a snapshot; discovery never writes source files.
type InstructionFile struct{ Path, Project, Text, Hash, Mtime string }
type InstructionSource struct {
	Home     string
	Sessions []Source
	// Workdirs, when set, carries each session's working directories across
	// calls so only sessions whose signature changed are read again.
	Workdirs *WorkdirCache
}

// WorkdirCache remembers the working directories of session units by unit
// signature. The zero value is ready to use.
type WorkdirCache struct {
	mu    sync.Mutex
	units map[string]workdirEntry
}

type workdirEntry struct {
	signature string
	dirs      []string
}

// unitWorkdirs returns u's absolute working directories, reading it only when
// the cache has no entry for its current signature.
func (c *WorkdirCache) unitWorkdirs(src Source, u Unit, next map[string]workdirEntry) ([]string, error) {
	key := fmt.Sprintf("%T\x00%s", src, u.Key)
	if c != nil {
		c.mu.Lock()
		e, ok := c.units[key]
		c.mu.Unlock()
		if ok && e.signature == u.Signature {
			next[key] = e
			return e.dirs, nil
		}
	}
	turns, err := src.Read(u)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var dirs []string
	for _, t := range turns {
		if !filepath.IsAbs(t.Workdir) {
			continue
		}
		if wd := filepath.Clean(t.Workdir); !seen[wd] {
			seen[wd] = true
			dirs = append(dirs, wd)
		}
	}
	next[key] = workdirEntry{u.Signature, dirs}
	return dirs, nil
}

var instructionImport = regexp.MustCompile(`(?:^|\s)@([^\s` + "`" + `]+)`)

func (s InstructionSource) Files() ([]InstructionFile, error) {
	home := s.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	projects := map[string]string{}
	next := map[string]workdirEntry{}
	// A source or session that cannot be read only loses its own projects:
	// session capture reports those errors, and instruction files found
	// through every other session are still captured.
	for _, src := range s.Sessions {
		units, err := src.Units()
		if err != nil {
			continue
		}
		for _, u := range units {
			dirs, err := s.Workdirs.unitWorkdirs(src, u, next)
			if err != nil {
				continue
			}
			for _, wd := range dirs {
				if old := projects[filepath.Dir(u.Key)]; old == "" || wd < old {
					projects[filepath.Dir(u.Key)] = wd
				}
				for p := wd; ; p = filepath.Dir(p) {
					if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
						projects["repo:"+wd] = p
						break
					}
					if filepath.Dir(p) == p {
						break
					}
				}
			}
		}
	}
	if s.Workdirs != nil {
		s.Workdirs.mu.Lock()
		s.Workdirs.units = next
		s.Workdirs.mu.Unlock()
	}
	seen := map[string]bool{}
	var out []InstructionFile
	var add func(string, string, int) error
	add = func(path, project string, depth int) error {
		if depth > InstructionImportDepth {
			return fmt.Errorf("instruction import depth exceeds %d", InstructionImportDepth)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		path, err = filepath.EvalSymlinks(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if seen[path] {
			return nil
		}
		seen[path] = true
		if len(seen) > InstructionMaxFiles {
			return fmt.Errorf("instruction file count exceeds %d", InstructionMaxFiles)
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err = f.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		b, err := io.ReadAll(io.LimitReader(f, InstructionMaxBytes+1))
		if err != nil {
			return err
		}
		if len(b) > InstructionMaxBytes {
			return fmt.Errorf("instruction file exceeds byte limit: %s", path)
		}
		hash := sha256.Sum256(b)
		out = append(out, InstructionFile{path, project, string(b), hex.EncodeToString(hash[:]), info.ModTime().UTC().Format(time.RFC3339Nano)})
		for _, match := range instructionImport.FindAllStringSubmatch(string(b), -1) {
			imp := strings.TrimRight(match[1], ".,;)")
			if strings.HasPrefix(imp, "~/") {
				imp = filepath.Join(home, imp[2:])
			} else if !filepath.IsAbs(imp) {
				imp = filepath.Join(filepath.Dir(path), imp)
			}
			if err := add(imp, project, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := add(filepath.Join(home, ".claude", "CLAUDE.md"), "", 0); err != nil {
		return nil, err
	}
	roots, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", "memory"))
	for _, root := range roots {
		project := projects[filepath.Dir(root)]
		if project == "" {
			project = filepath.Base(filepath.Dir(root))
		}
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() {
				return add(p, project, 0)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var keys []string
	for k := range projects {
		if strings.HasPrefix(k, "repo:") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		root, wd := projects[k], strings.TrimPrefix(k, "repo:")
		for p := wd; ; p = filepath.Dir(p) {
			for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
				if err := add(filepath.Join(p, name), root, 0); err != nil {
					return nil, err
				}
			}
			if p == root {
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
