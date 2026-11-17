package trace

import (
	"regexp"
	"strconv"
	"strings"
)

// Exit statuses as the harnesses print them at the head of a tool result:
// Claude Code's failed Bash ("Exit code 2"), Codex's unified exec envelope
// ({"chunk_id":…,"wall_time_seconds":…,"exit_code":1,…}) and its plain form
// ("Process exited with code 1", "Exit code: 1"), and PI's bash tool
// ("Command exited with code 1").
var (
	claudeExit = regexp.MustCompile(`\AExit code (-?\d+)\b`)
	codexExit  = regexp.MustCompile(`"wall_time_seconds":[-+0-9.eE]+,"exit_code":(-?\d+)`)
	plainExit  = regexp.MustCompile(`(?m)^(?:Process exited with code|Command exited with code|Exit code:) (-?\d+)\b`)
)

// shellTools are the tool names whose results carry a command's exit status.
func shellTool(name string) bool {
	switch strings.TrimPrefix(strings.ToLower(name), "functions.") {
	case "bash", "shell", "shell_command", "exec", "exec_command", "local_shell", "unified_exec":
		return true
	}
	return false
}

// CaptureToolOutcomes sets each tool result's ResultBytes and, where the
// harness reports one, its ExitCode; a Codex result whose script failed or
// whose command exited non-zero is marked IsError, because Codex records no
// error flag of its own. A value already set (by a parser that reads a
// native exit status, or by an earlier pass) is kept. It reads only the
// result's head, never stores result text anywhere new, and is idempotent.
func CaptureToolOutcomes(turns []Turn) []Turn {
	tools := map[string]string{}
	for ti := range turns {
		for bi := range turns[ti].Blocks {
			b := &turns[ti].Blocks[bi]
			switch b.Type {
			case "tool_use":
				if b.ToolUseID != "" {
					tools[b.ToolUseID] = b.ToolName
				}
			case "tool_result":
				name := b.ToolName
				if name == "" {
					name = tools[b.ToolUseID]
				}
				if b.ResultBytes == 0 {
					b.ResultBytes = int64(len(b.Text))
				}
				if b.ExitCode == nil {
					b.ExitCode = exitCode(turns[ti].Harness, name, b.Text, b.IsError)
				}
				if turns[ti].Harness == "codex" && (strings.HasPrefix(b.Text, "Script failed") || (b.ExitCode != nil && *b.ExitCode != 0)) {
					b.IsError = true
				}
			}
		}
	}
	return turns
}

func exitCode(harness, tool, text string, isError bool) *int {
	head, limit := text, 4096
	if harness == "codex" {
		limit = 1 << 16 // a script's later commands report further down
	}
	if len(head) > limit {
		head = head[:limit]
	}
	parse := func(s string) *int {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil
		}
		return &n
	}
	switch harness {
	case "claude_code":
		if !shellTool(tool) {
			return nil
		}
		if m := claudeExit.FindStringSubmatch(head); m != nil {
			return parse(m[1])
		}
		if !isError {
			zero := 0
			return &zero
		}
		return nil
	case "codex":
		// A script can run several commands: report the first non-zero
		// status, else zero.
		var code *int
		for _, m := range codexExit.FindAllStringSubmatch(head, -1) {
			if n := parse(m[1]); n != nil && (code == nil || (*code == 0 && *n != 0)) {
				code = n
			}
		}
		if code != nil {
			return code
		}
	}
	if m := plainExit.FindStringSubmatch(head); m != nil {
		return parse(m[1])
	}
	return nil
}
