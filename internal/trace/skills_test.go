package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillParserFixtures(t *testing.T) {
	fixtures := []struct {
		name, body string
		src        Source
	}{
		{"claude_code", `{"type":"assistant","sessionId":"s","uuid":"a","timestamp":"2026-10-29T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"Skill","input":{"skill":"review"}},{"type":"tool_use","name":"Read","input":{"file_path":"/home/test/.claude/skills/review/SKILL.md"}}]}}`, &ClaudeSource{}},
		{"codex", `{"type":"session_meta","payload":{"id":"s","cwd":"/repo"}}
{"type":"response_item","timestamp":"2026-10-29T10:00:00Z","payload":{"type":"function_call","id":"a","call_id":"c","name":"exec_command","arguments":"{\"cmd\":\"cat /home/test/.claude/skills/review/SKILL.md\"}"}}`, &CodexSource{}},
		{"pi", `{"type":"session","version":3,"id":"s","timestamp":"2026-10-29T10:00:00Z","cwd":"/repo"}
{"type":"message","id":"a","timestamp":"2026-10-29T10:00:00Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"c","name":"read","arguments":{"path":"/home/test/.claude/skills/review/SKILL.md"}}]}}`, &PiSource{}},
		{"dot", `{"session_id":"s","id":"a","ts":"2026-10-29T10:00:00Z","role":"assistant","tool_calls":[{"id":"c","name":"read_file","args":{"path":"/home/test/.claude/skills/review/SKILL.md"}}]}`, &DotSource{}},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "fixture.jsonl")
			if err := os.WriteFile(p, []byte(f.body+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			turns, err := f.src.Read(Unit{Key: p})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, turn := range turns {
				for _, b := range turn.Blocks {
					for _, s := range b.SkillLoads {
						if s.Name != "review" || s.TurnUUID != turn.TurnUUID || s.Seq != turn.Seq || s.TS != turn.TS {
							t.Fatalf("event %+v", s)
						}
						if s.Path != "" {
							found = true
							if s.Path != "/home/test/.claude/skills/review/SKILL.md" || s.Source != "/home/test/.claude/skills" {
								t.Fatalf("path %+v", s)
							}
						}
					}
				}
			}
			if !found {
				t.Fatalf("no path skill: %+v", turns)
			}
			sessions := Sessions(turns, nil, "")
			if len(sessions) != 1 || len(sessions[0].Skills) != 1 || sessions[0].Skills[0] != "review" {
				t.Fatalf("rollup %+v", sessions)
			}
		})
	}
}

func TestSkillReadBoundaries(t *testing.T) {
	for _, text := range []string{`read {"path":"/repo/skills/review/README.md"}`, `exec_command {"cmd":"echo /repo/skills/review/SKILL.md"}`, `read {"path":"/repo/docs/SKILL.md"}`} {
		turns := captureSkills([]Turn{{Blocks: []Block{{Type: "tool_use", ToolName: "exec_command", Text: text}}}})
		if len(turns[0].Blocks[0].SkillLoads) > 0 {
			t.Fatalf("false positive %s", text)
		}
	}
}
