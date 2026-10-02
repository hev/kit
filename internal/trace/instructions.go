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
}

var instructionImport = regexp.MustCompile(`(?:^|\s)@([^\s` + "`" + `]+)`)

func (s InstructionSource) Files() ([]InstructionFile, error) {
	home := s.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	projects := map[string]string{}
	for _, src := range s.Sessions {
		units, err := src.Units()
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, u := range units {
			turns, err := src.Read(u)
			if err != nil {
				return nil, err
			}
			for _, t := range turns {
				if !filepath.IsAbs(t.Workdir) {
					continue
				}
				wd := filepath.Clean(t.Workdir)
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
