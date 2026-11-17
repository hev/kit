package trace

import "testing"

// Exit statuses come only from harness metadata: envelopes and headers,
// whatever their key order and spacing, never from the output they wrap.
func TestExitCodeFromHarnessMetadataOnly(t *testing.T) {
	const none = -1
	script := func(items ...string) string {
		s := "Script completed\nWall time 0.1 seconds\nOutput:\n"
		for i, it := range items {
			if i > 0 {
				s += "\n"
			}
			s += it
		}
		return s
	}
	for _, tt := range []struct {
		name, harness, tool, text string
		isError                   bool
		want                      int
		wantErr                   bool
	}{
		// Review regression: valid JSON with spaces.
		{"spaced envelope", "codex", "exec", `{"wall_time_seconds": 0.1, "exit_code": 2, "output": "failed"}`, false, 2, true},
		{"spaced envelope in script", "codex", "exec", script(`{ "exit_code" : 3 , "chunk_id" : "a", "output" : "x" }`), false, 3, true},
		{"reordered keys", "codex", "exec", script(`{"output":"x","exit_code":4,"wall_time_seconds":0.2}`), false, 4, true},
		{"pretty printed", "codex", "exec", script("{\n  \"chunk_id\": \"a\",\n  \"wall_time_seconds\": 0.1,\n  \"exit_code\": 5,\n  \"output\": \"\"\n}"), false, 5, true},
		{"fulfilled wrapper", "codex", "exec", script(`{"status":"fulfilled","value":{"chunk_id":"a","wall_time_seconds":0.5,"exit_code":0,"output":"{\"exit_code\":9}"}}`), false, 0, false},
		{"session result wrapper", "codex", "exec", script(`{"session":1,"result":{"status":"fulfilled","value":{"chunk_id":"a","wall_time_seconds":1e-7,"exit_code":101,"output":""}}}`, `{"session":2,"result":{"status":"fulfilled","value":{"chunk_id":"b","wall_time_seconds":1e-7,"exit_code":0,"output":""}}}`), false, 101, true},
		{"array of envelopes", "codex", "wait", script(`[{"chunk_id":"a","wall_time_seconds":0.1,"exit_code":0,"output":""},{"chunk_id":"b","wall_time_seconds":0.1,"exit_code":7,"output":""}]`), false, 7, true},
		{"first non-zero wins", "codex", "exec", script(`{"chunk_id":"a","wall_time_seconds":0.1,"exit_code":0,"output":""}`, `{"chunk_id":"b","wall_time_seconds":0.1,"exit_code":1,"output":""}`, `{"chunk_id":"c","wall_time_seconds":0.1,"exit_code":2,"output":""}`), false, 1, true},
		{"truncated envelope", "codex", "exec", script("Warning: truncated output (original token count: 9)\nTotal output lines: 1\n\n" + `{"chunk_id":"a","wall_time_seconds":0.1,"exit_code":6,"output":"cut her`), false, 6, true},
		{"running session has no status", "codex", "exec", script(`{"chunk_id":"a","wall_time_seconds":1.0,"session_id":4,"output":"partial"}`), false, none, false},
		{"script failed", "codex", "exec", "Script failed\nWall time 0.0 seconds\nOutput:\nScript error:\nboom", false, none, true},
		// Command output a script printed raw is content.
		{"raw output in script", "codex", "exec", script("Exit code: 2\nProcess exited with code 3\nCommand exited with code 4"), false, none, false},
		{"object without envelope keys", "codex", "exec", script(`{"exit_code":2,"note":"a document"}`), false, none, false},
		{"escaped envelope in output", "codex", "exec", script(`{"chunk_id":"a","wall_time_seconds":0.1,"exit_code":0,"output":"{\"chunk_id\":\"z\",\"wall_time_seconds\":1,\"exit_code\":8}"}`), false, 0, false},
		// Review regression: a non-shell tool's content is never a status.
		{"read_file document", "codex", "read_file", "Exit code: 2\nThis is a document about exit codes.", false, none, false},
		{"view_image", "codex", "view_image", `{"chunk_id":"a","wall_time_seconds":0.1,"exit_code":3}`, false, none, false},
		{"non-shell script failed text", "codex", "read_file", "Script failed\nOutput:\n", false, none, false},
		// Codex direct shell tools: header before Output:, or JSON metadata.
		{"exec_command header", "codex", "exec_command", "Chunk ID: a\nWall time: 0.1 seconds\nProcess exited with code 2\nOriginal token count: 3\nOutput:\nx", false, 2, true},
		{"exec_command body is content", "codex", "exec_command", "Chunk ID: a\nWall time: 0.1 seconds\nProcess exited with code 0\nOutput:\nExit code: 9\nProcess exited with code 9", false, 0, false},
		{"exec_command still running", "codex", "write_stdin", "Chunk ID: a\nWall time: 1.0 seconds\nProcess running with session ID 4\nOutput:\nProcess exited with code 9", false, none, false},
		{"shell header", "codex", "shell", "Exit code: 1\nWall time: 0.2 seconds\nOutput:\nnope", false, 1, true},
		{"shell json metadata", "codex", "shell", `{"output":"Exit code: 9","metadata":{"exit_code": 0, "duration_seconds": 0.1}}`, false, 0, false},
		// Claude Code: Bash only, and only its leading marker.
		{"claude bash failed", "claude_code", "Bash", "Exit code 2\nboom", true, 2, true},
		{"claude bash ok", "claude_code", "Bash", "Exit code 2 appears in this file", false, 0, false},
		{"claude bash ok plain", "claude_code", "Bash", "fine\nExit code 3", false, 0, false},
		{"claude read", "claude_code", "Read", "Exit code 2\ncontent", false, none, false},
		// PI: bash's trailing line only.
		{"pi bash", "pi", "bash", "make: error\n\nCommand exited with code 2\n", true, 2, true},
		{"pi bash content mid", "pi", "bash", "Command exited with code 2\nmore output", false, none, false},
		{"pi read", "pi", "read", "notes\nCommand exited with code 2", false, none, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			turns := CaptureToolOutcomes([]Turn{{Harness: tt.harness, Blocks: []Block{
				{Type: "tool_use", ToolUseID: "x", ToolName: tt.tool},
				{Type: "tool_result", ToolUseID: "x", Text: tt.text, IsError: tt.isError},
			}}})
			b := turns[0].Blocks[1]
			got := none
			if b.ExitCode != nil {
				got = *b.ExitCode
			}
			if got != tt.want || b.IsError != tt.wantErr {
				t.Fatalf("exit=%d is_error=%v, want exit=%d is_error=%v", got, b.IsError, tt.want, tt.wantErr)
			}
			if b.ResultBytes != int64(len(tt.text)) {
				t.Fatalf("result_bytes=%d", b.ResultBytes)
			}
		})
	}
}
