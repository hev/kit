package main

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/hev/kit/skills"
)

// skillHarnesses are the agent homes `hev up` installs skills into, by the name
// a tick line gives them. A harness whose home does not exist is not installed
// on this machine and is left alone.
var skillHarnesses = []struct{ name, home string }{
	{"claude", ".claude"},
	{"codex", ".codex"},
}

// installSkills writes every embedded skill into each installed harness's
// skills directory, replacing what an older kit wrote there. A skill path that
// is a symlink is someone's working copy of the skill and is skipped.
func installSkills(home string) ([]string, error) {
	names, err := fs.ReadDir(skills.FS, ".")
	if err != nil {
		return nil, err
	}
	var installed []string
	for _, h := range skillHarnesses {
		root := filepath.Join(home, h.home)
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		for _, skill := range names {
			dest := filepath.Join(root, "skills", skill.Name())
			if fi, err := os.Lstat(dest); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if err := os.RemoveAll(dest); err != nil {
				return nil, err
			}
			sub, err := fs.Sub(skills.FS, skill.Name())
			if err != nil {
				return nil, err
			}
			if err := os.CopyFS(dest, sub); err != nil {
				return nil, err
			}
		}
		installed = append(installed, h.name)
	}
	return installed, nil
}
