package trace

import "testing"

func TestIndependentSkillProbe(t *testing.T) {
	for _, tt := range []struct {
		name, command string
		want          int
	}{
		{"codex-result-wrapper", `text(await tools.exec_command({cmd:"cat /repo/skills/actual/SKILL.md"}));`, 1},
		{"javascript-comment", "await tools.exec_command({cmd:\"echo ready\"}); // tools.exec_command({cmd:\"cat /repo/skills/false/SKILL.md\"})", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			turns := CaptureSkills([]Turn{{Harness: "codex", Blocks: []Block{{Type: "tool_use", ToolName: "exec", Text: "exec " + tt.command}}}})
			got := turns[0].Blocks[0].SkillLoads
			if len(got) != tt.want {
				t.Errorf("loads=%+v want=%d command=%s", got, tt.want, tt.command)
			}
		})
	}
}

func TestSkillJavaScriptWrappers(t *testing.T) {
	for _, tt := range []struct {
		name, code string
		want       int
	}{
		{"await", `await tools.exec_command({cmd:'cat "/repo with spaces/skills/actual/SKILL.md"'});`, 1},
		{"assignment", `const result = await tools.exec_command({cmd:"cat /repo/skills/actual/SKILL.md"});`, 1},
		{"parallel", `await Promise.allSettled([tools.exec_command({cmd:"cat /repo/skills/actual/SKILL.md"}), tools.exec_command({cmd:"echo /repo/skills/false/SKILL.md"})]);`, 1},
		{"pragma", "// @exec: {}\ntext(await tools.exec_command({cmd:'cat /repo/skills/actual/SKILL.md'}));", 1},
		{"block-comment", `/* tools.exec_command({cmd:"cat /repo/skills/false/SKILL.md"}) */ text(await tools.exec_command({cmd:"echo ready"}));`, 0},
		{"quoted-example", `text('tools.exec_command({cmd:"cat /repo/skills/false/SKILL.md"})');`, 0},
		{"unrelated-property", `text({cmd:"cat /repo/skills/false/SKILL.md"}); await tools.exec_command({cmd:"echo ready"});`, 0},
		{"shell-boundaries", `text(await tools.exec_command({cmd:"cat /repo/README.md; echo /repo/skills/false/SKILL.md"}));`, 0},
		{"shell-comment", `text(await tools.exec_command({cmd:"cat /repo/README.md # /repo/skills/false/SKILL.md"}));`, 0},
		{"dynamic-command", `await tools.exec_command({cmd:"cat /repo/skills/false/SKILL.md" + suffix});`, 0},
		{"escaped-quotes", `text(await tools.exec_command({cmd:"cat \"/repo with spaces/skills/actual/SKILL.md\""}));`, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			turns := CaptureSkills([]Turn{{Harness: "codex", TurnUUID: "js-turn", Seq: 3, TS: "2026-10-29T12:00:00Z", Blocks: []Block{{Type: "tool_use", ToolName: "exec", Text: "exec " + tt.code}}}})
			got := turns[0].Blocks[0].SkillLoads
			if len(got) != tt.want {
				t.Fatalf("loads=%+v want=%d", got, tt.want)
			}
			for _, load := range got {
				if load.Name != "actual" || load.TurnUUID != "js-turn" || load.Seq != 3 || load.TS != "2026-10-29T12:00:00Z" {
					t.Errorf("incorrect load: %+v", load)
				}
				if tt.name == "escaped-quotes" || tt.name == "await" {
					if load.Path != "/repo with spaces/skills/actual/SKILL.md" || load.Source != "/repo with spaces/skills" {
						t.Errorf("truncated path: %+v", load)
					}
				}
			}
		})
	}
}
