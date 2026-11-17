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
