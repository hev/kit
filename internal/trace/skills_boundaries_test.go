package trace

import "testing"

func TestSkillCommandBoundaryFixtures(t *testing.T) {
	for name, source := range map[string]Source{"claude_code": &ClaudeSource{}, "codex": &CodexSource{}, "pi": &PiSource{}, "dot": &DotSource{}} {
		t.Run(name, func(t *testing.T) {
			turns, err := source.Read(Unit{Key: "testdata/skills/" + name + "-boundaries.jsonl"})
			if err != nil {
				t.Fatal(err)
			}
			positive := map[string]bool{"spaces": true, "read-pipe": true, "awk": true, "rg": true, "sed": true, "multiline": true, "nl": true, "head": true, "tail": true, "less": true, "bat": true}
			var count int
			for _, turn := range turns {
				for _, block := range turn.Blocks {
					if block.Type != "tool_use" {
						continue
					}
					want := 0
					if positive[block.ToolUseID] {
						want = 1
					}
					if len(block.SkillLoads) != want {
						t.Fatalf("%s: loads=%+v want %d", block.ToolUseID, block.SkillLoads, want)
					}
					for _, load := range block.SkillLoads {
						count++
						root := "/repo/skills"
						if block.ToolUseID == "spaces" {
							root = "/repo with spaces/skills"
						}
						if load.Name != "actual" || load.Path != root+"/actual/SKILL.md" || load.Source != root || load.TurnUUID != turn.TurnUUID || load.Seq != turn.Seq || load.TS != turn.TS || load.TS == "" {
							t.Fatalf("%s: incorrect evidence %+v, turn %+v", block.ToolUseID, load, turn)
						}
					}
				}
			}
			if count != len(positive) {
				t.Fatalf("loads=%d want %d", count, len(positive))
			}
		})
	}
}

func TestClaudeNativeSkillOnlyFixture(t *testing.T) {
	turns, err := (&ClaudeSource{}).Read(Unit{Key: "testdata/skills/claude-native.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns=%d", len(turns))
	}
	for i, turn := range turns {
		if len(turn.Blocks) != 1 || len(turn.Blocks[0].SkillLoads) != 1 {
			t.Fatalf("missing native load: %+v", turn)
		}
		load := turn.Blocks[0].SkillLoads[0]
		want := "browser-use"
		if i == 1 {
			want = "review"
		}
		if load.Name != want || load.TurnUUID != turn.TurnUUID || load.TS != turn.TS {
			t.Fatalf("load=%+v", load)
		}
		if i == 1 && (load.Path != "/repo with spaces/skills/review/SKILL.md" || load.Source != "/repo with spaces/skills") {
			t.Fatalf("path evidence=%+v", load)
		}
	}
	rows := Sessions(turns, nil, "")
	if len(rows) != 1 || len(rows[0].Skills) != 2 {
		t.Fatalf("rollup=%+v", rows)
	}
}
