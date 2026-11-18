package trace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func loads(turns []Turn) []string {
	var out []string
	for _, turn := range turns {
		for _, b := range turn.Blocks {
			for _, l := range b.SkillLoads {
				out = append(out, l.Name+":"+l.Via)
			}
		}
	}
	return out
}

// A Skill-tool call and a SKILL.md read are both skill loads, in every
// harness, with how each was loaded.
func TestSkillLoadsEveryHarness(t *testing.T) {
	claude := writeFixture(t, "s.jsonl", strings.Join([]string{
		`{"type":"user","uuid":"u1","sessionId":"c1","timestamp":"2026-11-10T10:00:00Z","cwd":"/w","message":{"role":"user","content":"go"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"c1","timestamp":"2026-11-10T10:00:01Z","cwd":"/w","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Skill","input":{"skill":"kit:hev-board"}}]}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"a1","sessionId":"c1","timestamp":"2026-11-10T10:00:02Z","cwd":"/w","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/home/u/.claude/skills/peers/SKILL.md"}}]}}`,
		`{"type":"assistant","uuid":"a3","parentUuid":"a2","sessionId":"c1","timestamp":"2026-11-10T10:00:03Z","cwd":"/w","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"cat ~/.agents/skills/hev-dispatch/SKILL.md"}}]}}`,
	}, "\n")+"\n")
	codex := writeFixture(t, "rollout.jsonl", strings.Join([]string{
		`{"timestamp":"2026-11-10T10:00:00Z","type":"session_meta","payload":{"id":"x1","cwd":"/w"}}`,
		`{"timestamp":"2026-11-10T10:00:01Z","type":"response_item","payload":{"type":"function_call","id":"f1","call_id":"c1","name":"exec_command","arguments":"{\"cmd\":\"cat /home/u/.codex/skills/peers/SKILL.md /home/u/.agents/skills/hev-handoff/SKILL.md\"}"}}`,
		`{"timestamp":"2026-11-10T10:00:02Z","type":"response_item","payload":{"type":"function_call","id":"f2","call_id":"c2","name":"skill","arguments":"{\"name\":\"company\"}"}}`,
	}, "\n")+"\n")
	pi := writeFixture(t, "pi.jsonl", strings.Join([]string{
		`{"type":"session","version":3,"id":"p1","timestamp":"2026-11-10T10:00:00Z","cwd":"/w"}`,
		`{"type":"message","id":"e1","parentId":null,"timestamp":"2026-11-10T10:00:01Z","message":{"role":"user","content":"go"}}`,
		`{"type":"message","id":"e2","parentId":"e1","timestamp":"2026-11-10T10:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"k1","name":"read","arguments":{"path":"/home/u/.pi/agent/skills/browser-use/SKILL.md"}},{"type":"toolCall","id":"k2","name":"skill","arguments":{"skill":"oven:oven"}}]}}`,
	}, "\n")+"\n")
	for _, tt := range []struct {
		src  Source
		path string
		want []string
	}{
		{&ClaudeSource{}, claude, []string{"hev-board:tool", "peers:read", "hev-dispatch:read"}},
		{&CodexSource{}, codex, []string{"peers:read", "hev-handoff:read", "company:tool"}},
		{&PiSource{}, pi, []string{"browser-use:read", "oven:tool"}},
	} {
		turns, err := tt.src.Read(Unit{Key: tt.path})
		if err != nil {
			t.Fatal(err)
		}
		if got := loads(turns); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%T loads = %v, want %v", tt.src, got, tt.want)
		}
		rows := Sessions(turns, nil, "")
		if len(rows) != 1 || len(rows[0].SkillLoads) != len(tt.want) || rows[0].CaptureVersion != CaptureVersion {
			t.Fatalf("%T session skill loads: %+v", tt.src, rows)
		}
		for _, l := range rows[0].SkillLoads {
			if l.Via == "" || l.TurnUUID == "" || l.Path != "" {
				t.Fatalf("session load must carry name, via and turn only: %+v", l)
			}
		}
	}
}

func TestNormalizeSkillName(t *testing.T) {
	for in, want := range map[string]string{
		"peers":                        "peers",
		" /peers ":                     "peers",
		`"hev-board",`:                 "hev-board",
		`{"skill":"hev-dispatch"}`:     "hev-dispatch",
		`{"skill": "company", "args":`: "company",
		"code-review:code-review":      "code-review",
		"kit:hev-traces":               "hev-traces",
		"plugin:sub:name":              "name",
		`{"skill":`:                    "",
		"two words":                    "",
		"":                             "",
	} {
		if got := NormalizeSkillName(in); got != want {
			t.Errorf("NormalizeSkillName(%q) = %q, want %q", in, got, want)
		}
	}
	// A truncated Skill input still records its skill, never the wrapper.
	turns := CaptureSkills([]Turn{{TurnUUID: "a", Blocks: []Block{{Type: "tool_use", ToolName: "Skill", Text: `Skill {"skill":"peers", "args":"x`}}}})
	if got := loads(turns); !reflect.DeepEqual(got, []string{"peers:tool"}) {
		t.Fatalf("truncated input loads = %v", got)
	}
}

// Claude records the instruction files and skill catalogue it started with;
// Codex records the merged AGENTS.md and the skill catalogue. Only sizes and
// hashes are kept.
func TestStartContextFromTranscripts(t *testing.T) {
	claudeMD := "# Rules\nBe brief.\n"
	claude := writeFixture(t, "s.jsonl", strings.Join([]string{
		`{"type":"user","uuid":"u1","sessionId":"c1","timestamp":"2026-11-10T10:00:00Z","cwd":"/w","message":{"role":"user","content":"go"}}`,
		`{"type":"attachment","uuid":"x1","sessionId":"c1","timestamp":"2026-11-10T10:00:00Z","attachment":{"type":"instructions","files":[{"path":"/home/u/.claude/CLAUDE.md","type":"User","content":"# Rules\nBe brief.\n"}]}}`,
		`{"type":"attachment","uuid":"x2","sessionId":"c1","timestamp":"2026-11-10T10:00:00Z","attachment":{"type":"skill_listing","content":"- peers: talk\n- oven: run","skillCount":2,"isInitial":true}}`,
		`{"type":"attachment","uuid":"x3","sessionId":"c1","timestamp":"2026-11-10T10:00:09Z","attachment":{"type":"instructions","files":[{"path":"/w/sub/CLAUDE.md","type":"Project","content":"later"}]}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"c1","timestamp":"2026-11-10T10:00:01Z","cwd":"/w","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`,
	}, "\n")+"\n")
	turns, err := (&ClaudeSource{}).Read(Unit{Key: claude})
	if err != nil {
		t.Fatal(err)
	}
	rows := Sessions(turns, nil, "")
	c := rows[0].StartContext
	if c == nil || c.Source != "transcript" || len(c.Instructions) != 1 || c.Skills != 2 || c.SkillsSource != "transcript" {
		t.Fatalf("claude start context: %+v", c)
	}
	in := c.Instructions[0]
	if in.Path != "/home/u/.claude/CLAUDE.md" || in.Kind != "User" || in.Bytes != int64(len(claudeMD)) || in.SHA256 != ShortHash([]byte(claudeMD)) || len(in.SHA256) != 12 {
		t.Fatalf("instruction ref: %+v", in)
	}
	if c.SkillBytes != int64(len("- peers: talk\n- oven: run")) || rows[0].StartContextBytes != c.InstructionBytes+c.SkillBytes {
		t.Fatalf("sizes: %+v bytes=%d", c, rows[0].StartContextBytes)
	}

	agents := "Use peers.\n"
	codex := writeFixture(t, "rollout.jsonl", strings.Join([]string{
		`{"timestamp":"2026-11-10T10:00:00Z","type":"session_meta","payload":{"id":"x1","cwd":"/w"}}`,
		`{"timestamp":"2026-11-10T10:00:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<skills_instructions>\n## Skills\n- peers: talk (file: /h/.codex/skills/peers/SKILL.md)\n- oven: run (file: /h/.agents/skills/oven/SKILL.md)\n</skills_instructions>"}]}}`,
		`{"timestamp":"2026-11-10T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /w\n\n<INSTRUCTIONS>\nUse peers.\n</INSTRUCTIONS>"}]}}`,
		`{"timestamp":"2026-11-10T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"do it"}]}}`,
	}, "\n")+"\n")
	turns, err = (&CodexSource{}).Read(Unit{Key: codex})
	if err != nil {
		t.Fatal(err)
	}
	c = Sessions(turns, nil, "")[0].StartContext
	if c == nil || c.Skills != 2 || len(c.Instructions) != 1 || c.Instructions[0].Path != "/w/AGENTS.md" || c.Instructions[0].Bytes != int64(len(strings.TrimSuffix(agents, "\n"))) || c.Instructions[0].Kind != "merged" {
		t.Fatalf("codex start context: %+v", c)
	}
	for _, ref := range c.Instructions {
		if strings.Contains(ref.Path+ref.SHA256, "Use peers") {
			t.Fatal("content leaked into the record")
		}
	}
}

// Tool results carry is_error, an exit code where the harness reports one,
// and their size; the tool_use block row carries its result's outcome.
func TestToolOutcomes(t *testing.T) {
	claude := writeFixture(t, "s.jsonl", strings.Join([]string{
		`{"type":"assistant","uuid":"a1","sessionId":"c1","timestamp":"2026-11-10T10:00:01Z","cwd":"/w","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"false"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"true"}},{"type":"tool_use","id":"t3","name":"Read","input":{"file_path":"/x"}}]}}`,
		`{"type":"user","uuid":"u1","parentUuid":"a1","sessionId":"c1","timestamp":"2026-11-10T10:00:02Z","cwd":"/w","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"Exit code 2\nboom"},{"type":"tool_result","tool_use_id":"t2","content":"fine"},{"type":"tool_result","tool_use_id":"t3","is_error":true,"content":"File does not exist."}]}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"u1","sessionId":"c1","timestamp":"2026-11-10T10:00:03Z","cwd":"/w","message":{"role":"assistant","content":[{"type":"tool_use","id":"t4","name":"Edit","input":{"file_path":"/w/a"}}]}}`,
	}, "\n")+"\n")
	codex := writeFixture(t, "rollout.jsonl", strings.Join([]string{
		`{"timestamp":"2026-11-10T10:00:00Z","type":"session_meta","payload":{"id":"x1","cwd":"/w"}}`,
		`{"timestamp":"2026-11-10T10:00:01Z","type":"response_item","payload":{"type":"custom_tool_call","id":"f1","call_id":"c1","name":"exec","input":"tools.exec_command({cmd:\"hev board search ports\"})"}}`,
		`{"timestamp":"2026-11-10T10:00:02Z","type":"response_item","payload":{"type":"custom_tool_call_output","id":"o1","call_id":"c1","output":[{"type":"input_text","text":"Script completed\nWall time 0.1 seconds\nOutput:\n"},{"type":"input_text","text":"{\"chunk_id\":\"a\",\"wall_time_seconds\":0.1,\"exit_code\":1,\"output\":\"no\"}"}]}}`,
		`{"timestamp":"2026-11-10T10:00:03Z","type":"response_item","payload":{"type":"function_call","id":"f2","call_id":"c2","name":"exec_command","arguments":"{\"cmd\":\"ls\"}"}}`,
		`{"timestamp":"2026-11-10T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","id":"o2","call_id":"c2","output":"Chunk ID: b\nWall time: 0.1 seconds\nProcess exited with code 0\nOutput:\na.go"}}`,
		`{"timestamp":"2026-11-10T10:00:05Z","type":"response_item","payload":{"type":"custom_tool_call","id":"f3","call_id":"c3","name":"exec","input":"x"}}`,
		`{"timestamp":"2026-11-10T10:00:06Z","type":"response_item","payload":{"type":"custom_tool_call_output","id":"o3","call_id":"c3","output":"Script failed\nError: nope"}}`,
	}, "\n")+"\n")
	pi := writeFixture(t, "pi.jsonl", strings.Join([]string{
		`{"type":"session","version":3,"id":"p1","timestamp":"2026-11-10T10:00:00Z","cwd":"/w"}`,
		`{"type":"message","id":"e1","parentId":null,"timestamp":"2026-11-10T10:00:01Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"k1","name":"bash","arguments":{"command":"make"}}]}}`,
		`{"type":"message","id":"e2","parentId":"e1","timestamp":"2026-11-10T10:00:02Z","message":{"role":"toolResult","toolCallId":"k1","toolName":"bash","isError":true,"content":[{"type":"text","text":"make: error\n\nCommand exited with code 2"}]}}`,
	}, "\n")+"\n")
	type outcome struct {
		tool   string
		ok     bool
		exit   int64 // -1: none reported
		result bool
	}
	for _, tt := range []struct {
		src  Source
		path string
		want []outcome
	}{
		{&ClaudeSource{}, claude, []outcome{{"Bash", false, 2, true}, {"Bash", true, 0, true}, {"Read", false, -1, true}, {"Edit", true, -1, false}}},
		{&CodexSource{}, codex, []outcome{{"exec", false, 1, true}, {"exec_command", true, 0, true}, {"exec", false, -1, true}}},
		{&PiSource{}, pi, []outcome{{"bash", false, 2, true}}},
	} {
		turns, err := tt.src.Read(Unit{Key: tt.path})
		if err != nil {
			t.Fatal(err)
		}
		var got []outcome
		for _, b := range Blocks(turns) {
			if b.BlockType != "tool_use" {
				continue
			}
			o := outcome{b.ToolName, b.OK, -1, b.Resulted}
			if b.ExitCode != nil {
				o.exit = *b.ExitCode
			}
			if b.Resulted && b.ResultBytes == 0 {
				t.Fatalf("%T %s: result size missing", tt.src, b.ToolName)
			}
			got = append(got, o)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%T outcomes = %+v, want %+v", tt.src, got, tt.want)
		}
	}
	turns, _ := (&ClaudeSource{}).Read(Unit{Key: claude})
	s := Sessions(turns, nil, "")[0]
	if s.ToolResults != 3 || s.ToolErrors != 2 || s.ToolsBeforeEdit != 3 {
		t.Fatalf("claude session facts: results=%d errors=%d before-edit=%d", s.ToolResults, s.ToolErrors, s.ToolsBeforeEdit)
	}
	turns, _ = (&CodexSource{}).Read(Unit{Key: codex})
	s = Sessions(turns, nil, "")[0]
	if s.ToolErrors != 2 || s.ToolsBeforeEdit != -1 || s.CommandCounts["board_search"] != 1 {
		t.Fatalf("codex session facts: %+v", s)
	}
}

// Rows captured before capture version 1 still yield the same facts.
func TestSessionFactsFromBlockRows(t *testing.T) {
	turns := CaptureSkills([]Turn{
		{SessionID: "s", TurnUUID: "a", Role: "assistant", Harness: "codex", TS: "2026-11-10T10:00:00Z", Blocks: []Block{{Type: "tool_use", ToolName: "exec_command", ToolUseID: "c1", Text: "exec_command cmd: cat /h/.agents/skills/peers/SKILL.md && hev traces search x"}}},
		{SessionID: "s", TurnUUID: "b", Role: "tool", Harness: "codex", TS: "2026-11-10T10:00:01Z", Blocks: []Block{{Type: "tool_result", ToolUseID: "c1", Text: "Process exited with code 3"}}},
		{SessionID: "s", TurnUUID: "c", Role: "assistant", Harness: "codex", TS: "2026-11-10T10:00:02Z", Blocks: []Block{{Type: "tool_use", ToolName: "apply_patch", ToolUseID: "c2", Text: "apply_patch"}}},
	})
	rows := Blocks(turns)
	// Old rows: no outcome attributes, no skill loads.
	for i := range rows {
		rows[i].Resulted, rows[i].ExitCode, rows[i].ResultBytes, rows[i].SkillLoads, rows[i].OK = false, nil, 0, nil, true
	}
	facts := SessionFacts(TurnsFromBlocks(rows, Turn{SessionID: "s", Harness: "codex"}))
	if !reflect.DeepEqual([]string(facts.Skills), []string{"peers"}) || facts.ToolErrors != 1 || facts.ToolResults != 1 || facts.ToolsBeforeEdit != 1 || facts.CommandCounts["traces_search"] != 1 || facts.ToolCount != 2 {
		t.Fatalf("facts: %+v", facts)
	}
}
