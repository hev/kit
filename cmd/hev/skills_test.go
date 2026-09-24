package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInstallSkillsOnlyIntoInstalledHarnesses(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	got, err := installSkills(home)
	if err != nil || !reflect.DeepEqual(got, []string{"claude"}) {
		t.Fatalf("installed %v, err %v; want only claude", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "hev-query", "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Errorf("created a codex home that was not there: %v", err)
	}
}

func TestInstallSkillsReplacesStaleCopyButKeepsSymlink(t *testing.T) {
	home := t.TempDir()
	stale := filepath.Join(home, ".claude", "skills", "hev-query")
	os.MkdirAll(stale, 0o755)
	os.WriteFile(filepath.Join(stale, "old.md"), []byte("old"), 0o644)
	working := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex", "skills"), 0o755)
	os.Symlink(working, filepath.Join(home, ".codex", "skills", "hev-query"))

	if _, err := installSkills(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stale, "old.md")); !os.IsNotExist(err) {
		t.Error("stale file from an older kit survived")
	}
	if _, err := os.Stat(filepath.Join(working, "SKILL.md")); !os.IsNotExist(err) {
		t.Error("wrote through a symlinked working copy")
	}
}
