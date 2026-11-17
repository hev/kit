package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/skillsources"
)

func shelfTestRepo(t *testing.T, files map[string]string) (string, func(map[string]string) string) {
	t.Helper()
	r := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		b, err := exec.Command("git", append([]string{"-C", r}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	git("init", "-q", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.invalid")
	write := func(fs map[string]string) string {
		for p, s := range fs {
			p = filepath.Join(r, p)
			if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(s), 0644); err != nil {
				t.Fatal(err)
			}
		}
		git("add", ".")
		git("-c", "commit.gpgsign=false", "commit", "-q", "-m", "c")
		return git("rev-parse", "HEAD")
	}
	write(files)
	return r, write
}

// hev skills add and bump go through changeShelf; neither may move an
// ordinary skill source the command did not name (review finding P1).
func TestChangeShelfLeavesOrdinarySourcesAlone(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", config)
	t.Setenv("HEVLAYER_API_KEY", "")
	t.Setenv("HEVLAYER_API_KEY_REF", "")
	for _, d := range []string{".claude", ".codex"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	personal, advance := shelfTestRepo(t, map[string]string{"skills/personal/SKILL.md": "---\nname: personal\n---\nv1\n"})
	pack, _ := shelfTestRepo(t, map[string]string{
		"LICENSE":              "MIT License\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\n",
		"skills/plan/SKILL.md": "---\nname: plan\n---\nPlan.\n",
	})
	packCommit := strings.TrimSpace(func() string {
		b, err := exec.Command("git", "-C", pack, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}())
	old := "[skills]\n[[skills.sources]]\npath = " + `"` + filepath.ToSlash(personal) + `"` + "\nsubdir = \"skills\"\n"
	if err := os.WriteFile(config, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := skillsources.Load(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := skillsources.Run(home, cfg, true, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	links, err := skillsources.Installed(home)
	if err != nil || len(links) == 0 {
		t.Fatalf("links %v %v", links, err)
	}
	advance(map[string]string{"skills/personal/SKILL.md": "---\nname: personal\n---\nv2\n"})

	next, err := skillsources.AddSourceText(old, skillsources.Source{Name: "pack", Kind: skillsources.KindMode, URL: pack, Ref: "refs/heads/main", Subdir: "skills", Commit: packCommit})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := changeShelf(home, config, old, next, 0600, &out, &errOut); err != nil {
		t.Fatalf("%v\n%s%s", err, &out, &errOut)
	}
	for dest := range links {
		b, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
		if err != nil || !strings.Contains(string(b), "v1") {
			t.Fatalf("%s moved to %q (%v) on a shelf add\n%s", dest, b, err, &out)
		}
	}
	if after, _ := skillsources.Installed(home); len(after) != len(links) {
		t.Fatalf("links %v, want %v", after, links)
	}
	if packs, _ := skillsources.Packs(home); packs["pack"].Commit != packCommit {
		t.Fatalf("pack not installed: %+v", packs)
	}
}
