package trace

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestHarnessSkillFixtures(t *testing.T) {
	for _, tc := range []struct {
		harness string
		source  Source
		names   []string
		paths   []string
	}{
		{"claude_code", &ClaudeSource{}, []string{"board", "board", "review"}, []string{"", "/home/example/.claude/skills/board/SKILL.md", ""}},
		{"codex", &CodexSource{}, []string{"imagegen", "local", "board"}, []string{"/home/example/.codex/skills/.system/imagegen/SKILL.md", "/repo/skills/local/SKILL.md", "/home/example/.agents/skills/board/SKILL.md"}},
		{"pi", &PiSource{}, []string{"peers", "local"}, []string{"/home/example/.agents/skills/peers/SKILL.md", "/repo/skills/local/SKILL.md"}},
		{"dot", &DotSource{}, []string{"local"}, []string{"/repo/skills/local/SKILL.md"}},
	} {
		t.Run(tc.harness, func(t *testing.T) {
			turns, err := tc.source.Read(Unit{Key: "testdata/skills/" + tc.harness + ".jsonl"})
			if err != nil {
				t.Fatal(err)
			}
			var names, paths []string
			for _, turn := range turns {
				for _, b := range turn.Blocks {
					for _, load := range b.SkillLoads {
						names = append(names, load.Name)
						paths = append(paths, load.Path)
						if load.Turn != turn.TurnUUID || load.Seq != turn.Seq || load.Time != turn.TS || load.Time == "" || load.Commit != "" {
							t.Fatalf("incorrect provenance: %+v turn=%+v", load, turn)
						}
						if load.Path != "" {
							wantSource := strings.Split(load.Path, "/skills/")[0] + "/skills"
							if load.Source != wantSource {
								t.Fatalf("source=%q want=%q", load.Source, wantSource)
							}
						}
						wantTurn := "load"
						if tc.harness == "claude_code" && load.Name == "review" {
							wantTurn = "slash"
						}
						if tc.harness == "codex" {
							wantTurn = map[string]string{"imagegen": "load0", "local": "load5", "board": "wrapper"}[load.Name]
						}
						if load.Turn != wantTurn {
							t.Fatalf("turn=%s want=%s", load.Turn, wantTurn)
						}
					}
				}
			}
			if !reflect.DeepEqual(names, tc.names) || !reflect.DeepEqual(paths, tc.paths) {
				t.Fatalf("names=%v paths=%v", names, paths)
			}
			sessions := Sessions(turns, nil, "")
			wantNames := map[string]bool{}
			for _, name := range tc.names {
				wantNames[name] = true
			}
			unique := []string{}
			for name := range wantNames {
				unique = append(unique, name)
			}
			sort.Strings(unique)
			if len(sessions) != 1 || !reflect.DeepEqual([]string(sessions[0].Skills), unique) {
				t.Fatalf("rollup: %+v", sessions)
			}
			blocks := Blocks(turns)
			raw, err := json.Marshal(blocks)
			if err != nil {
				t.Fatal(err)
			}
			var roundtrip []BlockRow
			if err = json.Unmarshal(raw, &roundtrip); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(blocks, roundtrip) {
				t.Fatal("block events lost")
			}
		})
	}
}

func TestSkillReadBoundaries(t *testing.T) {
	for _, text := range []string{
		"cat /repo/README.md; echo /repo/skills/no/SKILL.md",
		"cat /repo/README.md > /repo/skills/no/SKILL.md",
		"cat /repo/skills/SKILL.md",
	} {
		turns := captureSkills([]Turn{{Blocks: []Block{{Type: "tool_use", ToolName: "exec_command", Text: text}}}})
		if len(turns[0].Blocks[0].SkillLoads) > 0 {
			t.Fatalf("false load: %q", text)
		}
	}
	if got := skillSource("/repo/notskills/foo/skills/board/SKILL.md"); got != "/repo/notskills/foo/skills" {
		t.Fatal(got)
	}
}
