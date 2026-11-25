package main

import (
	"bytes"
	"encoding/json"
	"github.com/hev/kit/skills"
	"io/fs"
	"mvdan.cc/sh/v3/shell"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestInstallSkillsOnlyIntoInstalledHarnesses(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	got, err := installSkills(home)
	if err != nil || !reflect.DeepEqual(got, []string{"claude"}) {
		t.Fatalf("installed %v, err %v; want only claude", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "hev-traces", "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Errorf("created a codex home that was not there: %v", err)
	}
}

func TestInstallSkillsReplacesStaleCopyButKeepsSymlink(t *testing.T) {
	home := t.TempDir()
	stale := filepath.Join(home, ".claude", "skills", "hev-traces")
	os.MkdirAll(stale, 0o755)
	old, err := os.ReadFile("testdata/hev-traces-unstamped.md")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stale, "SKILL.md"), old, 0o644)
	content, _ := skills.FS.ReadFile("hev-traces/SKILL.md")
	if got := inspectSkill(stale, content); got != "stale" {
		t.Fatalf("unstamped historical skill: %s", got)
	}
	os.WriteFile(filepath.Join(stale, "old.md"), []byte("old"), 0644)
	working := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex", "skills"), 0o755)
	os.Symlink(working, filepath.Join(home, ".codex", "skills", "hev-traces"))

	if _, err := installSkills(home); err != nil {
		t.Fatal(err)
	}
	if got := inspectSkill(stale, content); got != "current" {
		t.Fatalf("historical skill not repaired: %s", got)
	}
	if _, err := os.Stat(filepath.Join(stale, "old.md")); !os.IsNotExist(err) {
		t.Error("stale file from an older kit survived")
	}
	if _, err := os.Stat(filepath.Join(working, "SKILL.md")); !os.IsNotExist(err) {
		t.Error("wrote through a symlinked working copy")
	}
}

func TestSkillDriftRepair(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	if _, e := syncSkills(home, ownerAudience, true, false, nil); e != nil {
		t.Fatal(e)
	}
	stale := filepath.Join(home, ".claude", "skills", "hev-traces")
	raw, e := os.ReadFile(filepath.Join(stale, ".kit-skill.json"))
	if e != nil {
		t.Fatal(e)
	}
	var stamp skillStamp
	json.Unmarshal(raw, &stamp)
	stamp.Version = "old"
	raw, _ = json.Marshal(stamp)
	os.WriteFile(filepath.Join(stale, ".kit-skill.json"), raw, 0644)
	edited := filepath.Join(home, ".claude", "skills", "hev-board", "SKILL.md")
	os.WriteFile(edited, []byte("my edits"), 0644)
	var out bytes.Buffer
	if _, e = syncSkills(home, ownerAudience, false, false, &out); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"hev-traces: stale", "hev-board: locally edited"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal(out.String())
		}
	}
	if _, e = syncSkills(home, ownerAudience, true, false, nil); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(edited)
	if string(b) != "my edits" {
		t.Fatal("lost local edits")
	}
	content, _ := skills.FS.ReadFile("hev-traces/SKILL.md")
	if inspectSkill(stale, content) != "current" {
		t.Fatal("stale skill not repaired")
	}
	if _, e = syncSkills(home, ownerAudience, true, true, nil); e != nil {
		t.Fatal(e)
	}
	b, _ = skills.FS.ReadFile("hev-board/SKILL.md")
	dest := filepath.Dir(edited)
	if inspectSkill(dest, b) != "current" {
		t.Fatal("force did not repair")
	}
}

func TestBundledSkillCommands(t *testing.T) {
	if os.Getenv("KIT_SKILL_HELPER") == "1" {
		addDirectSurface()
		args, e := shell.Fields(os.Getenv("KIT_SKILL_COMMAND"), nil)
		if e != nil {
			os.Exit(1)
		}
		selected, _, e := rootCmd.Find(args)
		if e != nil || selected == rootCmd {
			os.Exit(1)
		}
		// Delegated backends can short-circuit --help before flag parsing.
		// Check their published flag vocabulary as well.
		if selected.Name() == "board" || selected.Name() == "peers" {
			allowed := map[string]bool{"--help": true, "-h": true}
			if selected.Name() == "board" {
				for _, f := range []string{"--board", "--tag", "--json", "--also", "--since", "--title", "-m", "-f", "--addr", "--namespace", "--to"} {
					allowed[f] = true
				}
			}
			if selected.Name() == "peers" {
				for _, f := range []string{"--json", "--peek", "--pending"} {
					allowed[f] = true
				}
			}
			for _, a := range args[1:] {
				if strings.HasPrefix(a, "-") && !allowed[strings.SplitN(a, "=", 2)[0]] {
					os.Exit(1)
				}
			}
		}
		rootCmd.SetArgs(append(args, "--help"))
		if e := rootCmd.Execute(); e != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	for _, line := range []string{"missing-command", "traces search query --missing-flag", "peers list --missing-flag", "board search query --missing-flag"} {
		c := exec.Command(os.Args[0], "-test.run=^TestBundledSkillCommands$")
		c.Env = append(os.Environ(), "KIT_SKILL_HELPER=1", "KIT_SKILL_COMMAND="+line)
		if out, e := c.CombinedOutput(); e == nil {
			t.Errorf("unknown command/flag accepted: %s\n%s", line, out)
		}
	}
	fs.WalkDir(skills.FS, ".", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			t.Fatal(e)
		}
		if d.IsDir() || !strings.HasSuffix(path, "SKILL.md") {
			return nil
		}
		b, e := skills.FS.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		normalized := strings.ReplaceAll(string(b), "\\\n", " ")
		for _, line := range strings.Split(normalized, "\n") {
			if !strings.HasPrefix(line, "hev ") {
				continue
			}
			// Help exercises command flags without executing pipelines or redirects.
			line = strings.SplitN(line, " |", 2)[0]
			line = strings.SplitN(line, " >", 2)[0]
			line = strings.TrimSuffix(strings.TrimSpace(line), "&")
			line = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(line, "example")
			c := exec.Command(os.Args[0], "-test.run=^TestBundledSkillCommands$")
			c.Env = append(os.Environ(), "KIT_SKILL_HELPER=1", "KIT_SKILL_COMMAND="+strings.TrimPrefix(line, "hev "))
			if out, e := c.CombinedOutput(); e != nil {
				t.Errorf("%s: %s: %v\n%s", path, line, e, out)
			}
		}
		return nil
	})
}

// The hev-peers skill ships the Stop/StopFailure wake hook; installing the
// skill writes it, and nothing touches the harness settings.
func TestPeersSkillShipsWakeHook(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	if _, err := installSkills(home); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "hev-peers", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(b), "```json\n")
	snippet, _, ok := strings.Cut(rest, "```")
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type, Command string
				AsyncRewake   bool `json:"asyncRewake"`
				Timeout       int
			}
		}
	}
	if !ok || json.Unmarshal([]byte(snippet), &doc) != nil {
		t.Fatalf("no JSON hook snippet:\n%s", b)
	}
	for _, event := range []string{"Stop", "StopFailure"} {
		groups := doc.Hooks[event]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("%s: %+v", event, groups)
		}
		h := groups[0].Hooks[0]
		if h.Type != "command" || !h.AsyncRewake || h.Timeout < 8*3600 || !strings.HasPrefix(h.Command, "hev peers inbox --wait --timeout ") {
			t.Fatalf("%s: %+v", event, h)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("skill install wrote settings.json: %v", err)
	}
}
