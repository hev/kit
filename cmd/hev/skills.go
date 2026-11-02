package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/version"
	"github.com/hev/kit/skills"
	"github.com/spf13/cobra"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var skillHarnesses = []struct{ name, home string }{{"claude", ".claude"}, {"codex", ".codex"}}

type skillStamp struct {
	Version string `json:"version"`
	Hash    string `json:"sha256"`
}
type skillState struct {
	Harness string `json:"harness"`
	Name    string `json:"name"`
	State   string `json:"state"`
}

func skillHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func inspectSkill(dest string, content []byte) string {
	if fi, e := os.Lstat(dest); e == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "locally edited"
	}
	b, e := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if os.IsNotExist(e) {
		return "missing"
	}
	if e != nil {
		return "locally edited"
	}
	var stamp skillStamp
	raw, e := os.ReadFile(filepath.Join(dest, ".kit-skill.json"))
	if os.IsNotExist(e) {
		historical, _ := skills.FS.ReadFile(filepath.Base(dest) + "/historical-sha256.txt")
		for _, hash := range strings.Fields(string(historical)) {
			if hash == skillHash(b) {
				return "stale"
			}
		}
	}
	if e != nil || json.Unmarshal(raw, &stamp) != nil {
		return "locally edited"
	}
	if stamp.Hash != skillHash(b) {
		return "locally edited"
	}
	if stamp.Version != version.Version || stamp.Hash != skillHash(content) {
		return "stale"
	}
	return "current"
}
func syncSkills(home string, repair, force bool, out io.Writer) ([]skillState, error) {
	names, err := fs.ReadDir(skills.FS, ".")
	if err != nil {
		return nil, err
	}
	var states []skillState
	for _, h := range skillHarnesses {
		root := filepath.Join(home, h.home)
		if fi, e := os.Stat(root); e != nil || !fi.IsDir() {
			continue
		}
		for _, skill := range names {
			if !skill.IsDir() {
				continue
			}
			content, e := skills.FS.ReadFile(skill.Name() + "/SKILL.md")
			if e != nil {
				return nil, e
			}
			dest := filepath.Join(root, "skills", skill.Name())
			state := inspectSkill(dest, content)
			if repair && state != "current" && (state != "locally edited" || force) {
				// Remove a symlink itself, never its target.
				if e = os.RemoveAll(dest); e != nil {
					return nil, e
				}
				if e = os.MkdirAll(dest, 0755); e != nil {
					return nil, e
				}
				if e = os.WriteFile(filepath.Join(dest, "SKILL.md"), content, 0644); e != nil {
					return nil, e
				}
				raw, _ := json.Marshal(skillStamp{version.Version, skillHash(content)})
				if e = os.WriteFile(filepath.Join(dest, ".kit-skill.json"), raw, 0644); e != nil {
					return nil, e
				}
				if state == "missing" {
					state = "installed"
				} else {
					state = "repaired"
				}
			}
			states = append(states, skillState{h.name, skill.Name(), state})
			if out != nil {
				fmt.Fprintf(out, "%s/%s: %s\n", h.name, skill.Name(), state)
			}
		}
	}
	return states, nil
}
func installSkills(home string) ([]string, error) {
	states, e := syncSkills(home, true, false, os.Stdout)
	var names []string
	for _, s := range states {
		if len(names) == 0 || names[len(names)-1] != s.Harness {
			names = append(names, s.Harness)
		}
	}
	return names, e
}
func init() {
	c := &cobra.Command{Use: "skills", Short: "Check and repair bundled agent skills"}
	for _, action := range []string{"check", "sync"} {
		repair := action == "sync"
		var force bool
		short := "Report bundled skill drift"
		if repair {
			short = "Install and repair bundled skills"
		}
		sub := &cobra.Command{Use: action, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			home, e := os.UserHomeDir()
			if e != nil {
				return e
			}
			states, e := syncSkills(home, repair, force, cmd.OutOrStdout())
			if e != nil {
				return e
			}
			if !repair {
				for _, state := range states {
					if state.State != "current" {
						return fmt.Errorf("bundled skills need synchronization")
					}
				}
			}
			return nil
		}}
		if repair {
			sub.Flags().BoolVar(&force, "force", false, "Replace locally edited skills (symlink targets are untouched)")
		}
		c.AddCommand(sub)
	}
	rootCmd.AddCommand(c)
}
