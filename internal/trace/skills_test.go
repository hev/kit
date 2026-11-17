package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSkillFixtures(t *testing.T) {
	tests := []struct {
		name   string
		source Source
		names  []string
		paths  []string
	}{
		{"claude_code", &ClaudeSource{}, []string{"board", "peers", "review"}, []string{"", "/home/test/.claude/skills/peers/SKILL.md", ""}},
		{"codex", &CodexSource{}, []string{"imagegen"}, []string{"/home/test/.codex/skills/.system/imagegen/SKILL.md"}},
		{"pi", &PiSource{}, []string{"peers"}, []string{"/home/test/.agents/skills/peers/SKILL.md"}},
		{"dot", &DotSource{}, []string{"local"}, []string{"/repo/skills/local/SKILL.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turns, err := tt.source.Read(Unit{Key: "testdata/skills/" + tt.name + ".jsonl"})
			if err != nil {
				t.Fatal(err)
			}
			var names, paths []string
			for _, turn := range turns {
				for _, b := range turn.Blocks {
					for _, s := range b.SkillLoads {
						names = append(names, s.Name)
						paths = append(paths, s.Path)
						if s.TurnUUID != turn.TurnUUID || s.Seq != turn.Seq || s.TS != turn.TS || s.TS == "" {
							t.Fatalf("lost location: %+v", s)
						}
						if s.Commit != "" {
							t.Fatal("invented historical commit")
						}
						if s.Path != "" && (s.Source == "" || strings.Contains(s.Source, ".system")) {
							t.Fatal("missing source")
						}
					}
				}
			}
			if !reflect.DeepEqual(names, tt.names) || !reflect.DeepEqual(paths, tt.paths) {
				t.Fatalf("names=%v paths=%v", names, paths)
			}
			rows := Sessions(turns, nil, "")
			if len(rows) != 1 || len(rows[0].Skills) != len(tt.names) {
				t.Fatalf("session skills: %+v", rows)
			}
			blocks := Blocks(turns)
			var count int
			for _, b := range blocks {
				count += len(b.SkillLoads)
			}
			if count != len(tt.names) {
				t.Fatalf("block events=%d", count)
			}
			before, _ := json.Marshal(turns)
			CaptureSkills(turns)
			after, _ := json.Marshal(turns)
			if string(before) != string(after) {
				t.Fatal("capture not idempotent")
			}
		})
	}
}

func TestSkillReadEvidence(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		want       int
	}{
		{"read", "read /repo/README.md", 0},
		{"read", "read /repo/SKILL.md", 0},
		{"read", "read /repo/nonskills/foo/SKILL.md", 0},
		{"bash", "bash rg --files /repo/skills/foo/SKILL.md", 0},
		{"bash", "bash echo 'cat /repo/skills/foo/SKILL.md'", 0},
		{"bash", "bash cat /repo/skills/foo/SKILL.md.backup", 0},
		{"exec_command", `exec_command {"cmd":"cat ~/.codex/skills/foo/SKILL.md"}`, 1},
		{"exec", `exec await tools.exec_command({cmd:"cat /repo/skills/foo/SKILL.md"})`, 1},
		{"bash", "bash cat /repo/skills/foo/SKILL.md /repo/skills/bar/SKILL.md", 2},
		{"bash", "bash cat /repo/skills/foo/SKILL.md; cat /repo/skills/foo/SKILL.md", 2},
		{"bash", "bash echo ready; cat /repo/skills/foo/SKILL.md", 1},
		{"bash", `bash echo 'await tools.exec_command({cmd:"cat /repo/skills/foo/SKILL.md"})'`, 0},
		{"bash", "bash grep -l heading /repo/skills/foo/SKILL.md", 1},
	} {
		turns := CaptureSkills([]Turn{{Blocks: []Block{{Type: "tool_use", ToolName: tt.name, Text: tt.text}}}})
		if got := len(turns[0].Blocks[0].SkillLoads); got != tt.want {
			t.Errorf("%q: got %d want %d", tt.text, got, tt.want)
		}
	}
	turns := CaptureSkills([]Turn{{Blocks: []Block{{Type: "text", Text: "cat /repo/skills/foo/SKILL.md"}, {Type: "tool_result", Text: "/repo/skills/foo/SKILL.md"}}}})
	for _, b := range turns[0].Blocks {
		if len(b.SkillLoads) > 0 {
			t.Fatal("catalogue/output counted as load")
		}
	}
}

// A load whose path is in a pinned shelf snapshot, directly or through a run's
// live link into one, records the pinned commit; other loads stay empty.
func TestSkillCaptureFillsPinnedCommit(t *testing.T) {
	sha := strings.Repeat("c", 40)
	home := t.TempDir()
	pin := filepath.Join(home, ".local", "opt", "hev-skills-0123456789abcdef-"+sha, "pack", "skills", "plan")
	if err := os.MkdirAll(pin, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(pin, "SKILL.md"), []byte("plan\n"), 0644)
	plain := filepath.Join(home, "repo", ".claude", "skills", "house")
	os.MkdirAll(plain, 0755)
	os.WriteFile(filepath.Join(plain, "SKILL.md"), []byte("house\n"), 0644)
	linked := filepath.Join(home, "wt", ".claude", "skills")
	os.MkdirAll(linked, 0755)
	symlinked := os.Symlink(pin, filepath.Join(linked, "plan")) == nil
	read := func(p string) Block {
		b, _ := json.Marshal(map[string]string{"file_path": filepath.ToSlash(p)})
		return Block{Type: "tool_use", ToolName: "Read", Text: string(b)}
	}
	turns := CaptureSkills([]Turn{{Blocks: []Block{
		read(filepath.Join(pin, "SKILL.md")),
		read(filepath.Join(plain, "SKILL.md")),
		read(filepath.Join(linked, "plan", "SKILL.md")),
		read("/gone/.local/opt/hev-skills-0123456789abcdef-" + sha + "/skills/x/SKILL.md"),
	}}})
	got := []string{}
	for _, b := range turns[0].Blocks {
		if len(b.SkillLoads) != 1 {
			t.Fatalf("loads %+v", b.SkillLoads)
		}
		got = append(got, b.SkillLoads[0].Commit)
	}
	want := []string{sha, "", sha, sha}
	if !symlinked {
		want[2] = ""
	}
	if filepath.Separator == '\\' {
		want[0], want[2] = "", ""
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commits %q, want %q", got, want)
	}
}
