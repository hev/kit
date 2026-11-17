package trace

import (
	"reflect"
	"testing"
)

// Command facts count executed command nodes only. Each case is one tool
// call; want is its command_counts and whether it is an edit.
func TestSessionFactsCountExecutedCommands(t *testing.T) {
	cases := []struct {
		name, harness, tool, text string
		want                      map[string]int
		edit                      bool
	}{
		// Negative: the words are data, not commands.
		{"codex printf of quoted examples", "codex", "exec_command", "exec_command cmd: printf '%s\\n' 'hev board search example; hev traces search example; hev dispatch example; git commit; apply_patch'", nil, false},
		{"claude echo double-quoted", "claude_code", "Bash", `Bash echo "run hev board search ports, then git commit"`, nil, false},
		{"heredoc body written to a doc", "claude_code", "Bash", "Bash cat > docs/x.md <<'EOF'\nhev traces search foo\nhev dispatch bar\ngit commit -m x\napply_patch\nEOF", nil, false},
		{"comment", "claude_code", "Bash", "Bash ls # hev board search later", nil, false},
		{"grep for the command text", "claude_code", "Bash", `Bash grep -rn "hev dispatch" docs/`, nil, false},
		{"git log mentioning commit", "claude_code", "Bash", "Bash git log --grep commit", nil, false},
		{"pi json command printing", "pi", "bash", `bash {"command":"echo 'hev board post x' && echo apply_patch"}`, nil, false},
		{"codex code mode string not executed", "codex", "exec", `exec const s = "hev board search x; apply_patch"; text(s)`, nil, false},
		{"codex code mode comment", "codex", "exec", "exec // tools.apply_patch(\"x\")\ntools.exec_command({cmd: \"echo 'hev dispatch y'\"})", nil, false},
		{"non-shell tool", "claude_code", "Read", "Read /x/hev board search.md", nil, false},
		{"unparseable script", "claude_code", "Bash", "Bash hev board search 'unterminated", nil, false},

		// Positive: executed commands.
		{"claude chain", "claude_code", "Bash", "Bash cd /x && hev board search ports | head; hev traces search 'flaky test'", map[string]int{"board_search": 1, "traces_search": 1}, false},
		{"global flag and wrappers", "claude_code", "Bash", "Bash FOO=1 timeout 30 hev --json board post note; env A=b hev board reply 1 ok", map[string]int{"board_post": 2}, false},
		{"bash -lc script", "codex", "exec_command", "exec_command cmd: bash -lc 'hev dispatch send order.md && git -C wt commit -m done'\nworkdir: /tmp", map[string]int{"dispatch": 1}, true},
		{"command substitution", "claude_code", "Bash", `Bash out=$(hev traces search x)`, map[string]int{"traces_search": 1}, false},
		{"apply_patch heredoc", "codex", "exec_command", "exec_command cmd: apply_patch <<'EOF'\n*** Begin Patch\n*** hev dispatch in patch text\n*** End Patch\nEOF", nil, true},
		{"pi bash json", "pi", "bash", `bash {"command":"hev board search \"x y\""}`, map[string]int{"board_search": 1}, false},
		{"codex code mode exec_command", "codex", "exec", "exec tools.exec_command({cmd: \"hev board search ports\"})", map[string]int{"board_search": 1}, false},
		{"codex code mode apply_patch", "codex", "exec", "exec await tools.apply_patch(\"*** Begin Patch\\n*** End Patch\")", nil, true},
		{"edit tool", "claude_code", "Edit", "Edit /x/y.go", nil, true},
		{"codex code mode template literal", "codex", "exec", "exec await Promise.all(names.map(async n => text(await tools.exec_command({cmd: `/opt/bin/hev dispatch show ${n.name} --json`, max_output_tokens: 2000}))))", map[string]int{"dispatch": 1}, false},
		{"codex code mode command list", "codex", "exec", "exec const cmds=[\"peers\",\"hev board search 'promo loop'\",\"cat notes.md\"];\nfor (const c of cmds) text(await tools.exec_command({cmd: c}))", map[string]int{"board_search": 1}, false},
		{"codex code mode list passed whole", "codex", "exec", "exec const r = await Promise.allSettled([\"hev traces search x\", \"ls\"].map(c => tools.exec_command({cmd: c, yield_time_ms: 1000})))", map[string]int{"traces_search": 1}, false},
		{"codex code mode shorthand cmd", "codex", "exec", "exec const r=await Promise.allSettled([\"hev dispatch show a --json\",\"hev dispatch show b --json\"].map(cmd=>tools.exec_command({cmd,max_output_tokens:4500})));r.forEach(text);", map[string]int{"dispatch": 2}, false},
		{"codex code mode call objects", "codex", "exec", "exec const calls=[[\"peers\",{cmd:\"peers\",max_output_tokens:2500}],[\"board\",{cmd:\"hev board search promo\",max_output_tokens:2500}]];\nfor (const [k,o] of calls) text(await tools.exec_command(o))", map[string]int{"board_search": 1}, false},
		{"codex code mode data list with literal cmd", "codex", "exec", "exec const notes=[\"hev board search x\"]; text(await tools.exec_command({cmd: \"ls\"}))", nil, false},
		{"codex code mode echo in template", "codex", "exec", "exec await tools.exec_command({cmd: `echo 'hev dispatch ${x}'`})", nil, false},
		// Negative: literals beside an indirect call that do not reach it.
		{"codex indirect cmd beside unrelated array", "codex", "exec", "exec const notes = [\"hev board search example; hev traces search example; hev dispatch example; git commit; apply_patch\"]; const cmd = \"ls\"; text(notes); text(await tools.exec_command({cmd}));", nil, false},
		{"codex indirect cmd beside unrelated object", "codex", "exec", "exec const example = {cmd: \"hev dispatch send x\"}; const c = \"ls\"; text(example); text(await tools.exec_command({cmd: c}))", nil, false},
		{"codex loop over one list beside another", "codex", "exec", "exec const docs = [\"hev board search a\", \"git commit\"]; const cmds = [\"ls\", \"pwd\"]; text(docs); for (const c of cmds) text(await tools.exec_command({cmd: c}))", nil, false},
		{"codex object arg beside unrelated objects", "codex", "exec", "exec const shown = [{cmd: \"hev traces search x\"}]; const o = {cmd: \"ls\"}; text(shown); await tools.exec_command(o)", nil, false},
		{"codex map over one list beside another", "codex", "exec", "exec const notes = [\"hev dispatch a\"]; await Promise.all([\"ls\"].map(c => tools.exec_command({cmd: c})))", nil, false},
		{"codex computed cmd stays unknown", "codex", "exec", "exec const base = \"hev board search \"; const words = [\"hev dispatch x\"]; await tools.exec_command({cmd: base + q})", nil, false},
		{"codex function parameter stays unknown", "codex", "exec", "exec const list = [\"hev dispatch x\"]; async function run(c) { return tools.exec_command({cmd: c}) } text(list)", nil, false},
		// Positive: a literal and an indirect call each count once.
		{"codex mixed literal and indirect", "codex", "exec", "exec await tools.exec_command({cmd:\"hev board search x\"}); const cmds=[\"ls\"]; for(const cmd of cmds) await tools.exec_command({cmd});", map[string]int{"board_search": 1}, false},
		{"codex declared object arg", "codex", "exec", "exec const o = {cmd: \"hev traces search x\", yield_time_ms: 10}; await tools.exec_command(o)", map[string]int{"traces_search": 1}, false},
		{"codex member cmd over objects", "codex", "exec", "exec const jobs = [{cmd: \"hev dispatch show a\"}, {cmd: \"ls\"}]; for (const j of jobs) await tools.exec_command({cmd: j.cmd})", map[string]int{"dispatch": 1}, false},
		{"codex destructured arrow over tuples", "codex", "exec", "exec const calls = [[\"peers\", \"peers\"], [\"board\", \"hev board search promo\"], [\"note\", \"cat x\"]];\nconst r = await Promise.allSettled(calls.map(async ([name, cmd]) => ({name, ...(await tools.exec_command({cmd, workdir: \"/w\"}))})))", map[string]int{"board_search": 1}, false},
		{"codex arrow with index parameter", "codex", "exec", "exec const cmds = [\"peers\", \"hev board search promo\"]; await Promise.allSettled(cmds.map((cmd, i) => tools.exec_command({cmd, max_output_tokens: i === 0 ? 10 : 20})))", map[string]int{"board_search": 1}, false},
		{"codex destructured arrow picks its element", "codex", "exec", "exec const jobs = [[\"hev dispatch label\", \"ls\"]]; await Promise.all(jobs.map(async ([label, cmd]) => tools.exec_command({cmd})))", nil, false},
		{"codex index parameter is not the element", "codex", "exec", "exec const cmds = [\"hev dispatch x\"]; await Promise.all(cmds.map((x, cmd) => tools.exec_command({cmd})))", nil, false},
		{"codex nested destructuring", "codex", "exec", "exec const jobs = [[\"peers\", [\"peers\"]], [\"board\", [\"hev board search x\"]]].map(async ([k, [cmd]]) => [k, await tools.exec_command({cmd})])", map[string]int{"board_search": 1}, false},
		{"codex for-of nested destructuring", "codex", "exec", "exec for (const [label, [cmd, note]] of [[\"a\", [\"hev traces search x\", \"hev dispatch note\"]]]) await tools.exec_command({cmd})", map[string]int{"traces_search": 1}, false},
		{"codex template in a loop counts once", "codex", "exec", "exec await Promise.all([\"a\", \"b\"].map(async n => text(await tools.exec_command({cmd: `hev dispatch show ${n}`}))))", map[string]int{"dispatch": 1}, false},
		{"codex parenthesised async arrow", "codex", "exec", "exec const cmds = [\"hev board post x\"]; await Promise.all(cmds.map(async (c) => tools.exec_command({cmd: c})))", map[string]int{"board_post": 1}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := SessionFacts([]Turn{{Harness: c.harness, Role: "assistant", Blocks: []Block{{Type: "tool_use", ToolName: c.tool, Text: c.text}}}})
			want := ToolCounts{}
			for k, v := range c.want {
				want[k] = v
			}
			if !reflect.DeepEqual(s.CommandCounts, want) {
				t.Errorf("command_counts = %v, want %v", s.CommandCounts, want)
			}
			if wantEdit := map[bool]int64{true: 0, false: -1}[c.edit]; s.ToolsBeforeEdit != wantEdit {
				t.Errorf("tools_before_edit = %d, want %d", s.ToolsBeforeEdit, wantEdit)
			}
		})
	}
}

// The review's probe: printing examples is not running them.
func TestQuotedCommandExamplesAreNotActivity(t *testing.T) {
	s := SessionFacts([]Turn{{Harness: "codex", Blocks: []Block{{Type: "tool_use", ToolName: "exec_command", Text: "exec_command\ncmd: printf '%s\\n' 'hev board search example; hev traces search example; hev dispatch example; git commit; apply_patch'"}}}})
	if len(s.CommandCounts) != 0 || s.ToolsBeforeEdit != -1 {
		t.Fatalf("printing command examples counted as activity: commands=%v tools_before_edit=%d", s.CommandCounts, s.ToolsBeforeEdit)
	}
}

// Old rows are derived through TurnsFromBlocks and SessionFacts: the same
// rule applies.
func TestDerivedFactsIgnoreQuotedCommands(t *testing.T) {
	rows := []BlockRow{
		{SessionID: "s", TurnUUID: "a", Seq: 0, Role: "assistant", BlockType: "tool_use", ToolName: "Bash", ToolUseID: "1", Text: `Bash echo "hev board search x && git commit"`},
		{SessionID: "s", TurnUUID: "a", Seq: 1, Role: "assistant", BlockType: "tool_use", ToolName: "Bash", ToolUseID: "2", Text: `Bash hev board search x`},
		{SessionID: "s", TurnUUID: "a", Seq: 2, Role: "assistant", BlockType: "tool_use", ToolName: "Bash", ToolUseID: "3", Text: `Bash git commit -am x`},
	}
	s := SessionFacts(TurnsFromBlocks(rows, Turn{SessionID: "s", Harness: "claude_code"}))
	if s.CommandCounts["board_search"] != 1 || len(s.CommandCounts) != 1 || s.ToolsBeforeEdit != 2 {
		t.Fatalf("derived: commands=%v before_edit=%d", s.CommandCounts, s.ToolsBeforeEdit)
	}
}

// The final review's probes, kept as regressions.
func TestIndirectCommandsIgnoreUnrelatedData(t *testing.T) {
	s := SessionFacts([]Turn{{Harness: "codex", Blocks: []Block{{Type: "tool_use", ToolName: "exec", Text: `exec const notes = ["hev board search example; hev traces search example; hev dispatch example; git commit; apply_patch"]; const cmd = "ls"; text(notes); text(await tools.exec_command({cmd}));`}}}})
	if len(s.CommandCounts) != 0 || s.ToolsBeforeEdit != -1 {
		t.Fatalf("unexecuted data counted: commands=%v tools_before_edit=%d", s.CommandCounts, s.ToolsBeforeEdit)
	}
}

func TestMixedLiteralAndIndirectCountOnce(t *testing.T) {
	s := SessionFacts([]Turn{{Harness: "codex", Blocks: []Block{{Type: "tool_use", ToolName: "exec", Text: `exec await tools.exec_command({cmd:"hev board search x"}); const cmds=["ls"]; for(const cmd of cmds) await tools.exec_command({cmd});`}}}})
	if s.CommandCounts["board_search"] != 1 {
		t.Fatalf("commands=%v want one board_search", s.CommandCounts)
	}
}
