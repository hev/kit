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
				for _, f := range []string{"--board", "--tag", "--json", "--also", "--since", "--title", "-m", "-f", "--addr", "--namespace", "--to", "--image", "--images"} {
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

// The context review skill is bundled for owners, describes plan → propose →
// one-screen review → dispatch, never edits instruction files, and
// hev-dispatch routes to it with the no-files and skip cases.
func TestContextReviewSkill(t *testing.T) {
	b, err := skills.FS.ReadFile("hev-context/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := skillTargeted("hev-context", b, ownerAudience); err != nil || !ok {
		t.Fatalf("not for owners: %v", err)
	}
	if ok, _ := skillTargeted("hev-context", b, skillAudience{Member: true}); ok {
		t.Fatal("members dispatch nothing; the review is owner-only")
	}
	text := string(b)
	for _, want := range []string{"hev context plan", "hev context build", "--context-file", "one screen", "yes or an edit", "digest", "never edit a repository's instruction", "Don't offer or open a change", "**Skip.**", "nothing to narrow"} {
		if !strings.Contains(text, want) {
			t.Errorf("hev-context lacks %q", want)
		}
	}
	d, _ := skills.FS.ReadFile("hev-dispatch/SKILL.md")
	for _, want := range []string{"`hev-context` skill", "finds no instruction files", "says to skip the review", "never prompts"} {
		if !strings.Contains(string(d), want) {
			t.Errorf("hev-dispatch lacks %q", want)
		}
	}
}

// Updating hev-dispatch and hev-peers for steering keeps their shipped copies
// recognizable: an unstamped earlier SKILL.md is stale, not someone else's.
func TestSteeringSkillsKeepHistoricalHashes(t *testing.T) {
	for skill, old := range map[string]string{"hev-dispatch": "59090c0058b8c3aebfa4cff9b904b89274ee6a188d7f9ee42c90e16e28e7643e", "hev-peers": "e3163f523c6fb58bd8a470fe1d478e14255f55a80a8a6278d2404d37c07049ff"} {
		b, err := skills.FS.ReadFile(skill + "/historical-sha256.txt")
		if err != nil || !strings.Contains(string(b), old) {
			t.Fatalf("%s: pre-steering hash missing: %v", skill, err)
		}
		current, _ := skills.FS.ReadFile(skill + "/SKILL.md")
		for _, want := range []string{"hev steer", "--level urgent", "hev steer ack"} {
			if !strings.Contains(string(current), want) {
				t.Errorf("%s does not teach %q", skill, want)
			}
		}
	}
}

// An unstamped hev-dispatch copy from the v0.7 base this branch merged
// (4aecce0, before steering) is kit content: sync replaces it with the
// steering-aware skill instead of reporting it as someone else's.
func TestSyncRepairsUnstampedBaseDispatchSkill(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".claude", "skills", "hev-dispatch")
	os.MkdirAll(dest, 0o755)
	old, err := os.ReadFile("testdata/hev-dispatch-unstamped.md")
	if err != nil {
		t.Fatal(err)
	}
	if got := skillHash(old); got != "11c36b66f8288f074dfa98317f4d4453bb4bd52e5b79bc33f673fa8bb2afdab8" {
		t.Fatalf("fixture is not the 4aecce0 blob: %s", got)
	}
	if strings.Contains(string(old), "hev steer") {
		t.Fatal("fixture already teaches hev steer")
	}
	os.WriteFile(filepath.Join(dest, "SKILL.md"), old, 0o644)
	content, _ := skills.FS.ReadFile("hev-dispatch/SKILL.md")
	if got := inspectSkill(dest, content); got != "stale" {
		t.Fatalf("unstamped base dispatch skill: %s", got)
	}
	var out bytes.Buffer
	if _, err := syncSkills(home, ownerAudience, true, false, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "hev-dispatch: "+notHevs) {
		t.Fatalf("sync treated kit content as foreign:\n%s", out.String())
	}
	if got := inspectSkill(dest, content); got != "current" {
		t.Fatalf("base dispatch skill not repaired: %s\n%s", got, out.String())
	}
	b, _ := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if !bytes.Equal(b, content) || !strings.Contains(string(b), "hev steer") {
		t.Fatal("sync did not install the steering-aware hev-dispatch skill")
	}
}

// The files-board change to hev-board keeps the copy v0.7 shipped before it
// recognizable, so sync updates an unstamped one instead of leaving it.
func TestBoardSkillKeepsHistoricalHashes(t *testing.T) {
	b, err := skills.FS.ReadFile("hev-board/historical-sha256.txt")
	if err != nil || !strings.Contains(string(b), "71bb3e5dc9dce3d6dfc19b69440b796f1d23102f803c0e35638dd23014c3c0f5") {
		t.Fatalf("hev-board: pre-files-board hash missing: %v", err)
	}
	current, _ := skills.FS.ReadFile("hev-board/SKILL.md")
	if !strings.Contains(string(current), "search is\ngrep") {
		t.Error("hev-board does not say Starter Kit search is grep")
	}
}
