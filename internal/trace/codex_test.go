package trace

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCodexUnitsUsePathAndSizeMtimeSignature(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "2026", "09", "05")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "rollout.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	units, err := (&CodexSource{Root: root}).Units()
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Key != path {
		t.Fatalf("units = %+v", units)
	}
	want := strings.Join([]string{fmtInt(info.Size()), fmtInt(info.ModTime().UnixNano())}, "-")
	if units[0].Signature != want {
		t.Fatalf("signature = %q, want %q", units[0].Signature, want)
	}
}

func fmtInt(v int64) string { return strconv.FormatInt(v, 10) }

func TestCodexReadNormalizesLegacyAndCurrentRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	lines := []string{
		`{"timestamp":"2026-09-05T10:00:00Z","type":"session_meta","payload":{"id":"session-1","cli_version":"0.152.1","cwd":"/w/repo"}}`,
		`{"timestamp":"2026-09-05T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"make the change","turn_id":"turn-1"}}`,
		`{"timestamp":"2026-09-05T10:00:01.5Z","type":"turn_context","payload":{"turn_id":"request-1","model":"gpt-5.6-sol","effort":"high","cwd":"/w/repo"}}`,
		`{"timestamp":"2026-09-05T10:00:02Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"inspect first","turn_id":"turn-1"}}`,
		`{"timestamp":"2026-09-05T10:00:03Z","type":"response_item","payload":{"type":"message","id":"msg-1","role":"assistant","content":[{"type":"output_text","text":"I will inspect it"}]}}`,
		`{"timestamp":"2026-09-05T10:00:04Z","type":"response_item","payload":{"type":"function_call","id":"call-item","call_id":"call-1","name":"exec_command","arguments":"{\"cmd\":\"go test ./...\"}"}}`,
		`{"timestamp":"2026-09-05T10:00:05Z","type":"response_item","payload":{"type":"function_call_output","id":"output-1","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-09-05T10:00:05.5Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":80,"cache_write_input_tokens":5,"output_tokens":20}}}}`,
		`{"timestamp":"2026-09-05T10:00:06Z","type":"response_item","payload":{"type":"message","id":"dev-1","role":"developer","content":[{"type":"input_text","text":"do not index setup"}]}}`,
		`{"timestamp":"2026-09-05T10:00:07Z","type":"event_msg","payload":{"type":"token_count"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	turns, err := (&CodexSource{}).Read(Unit{Key: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 5 {
		t.Fatalf("turns = %d, want 5: %+v", len(turns), turns)
	}
	for i, turn := range turns {
		if turn.SessionID != "session-1" || turn.Workdir != "/w/repo" || turn.Harness != "codex" || turn.SourcePath != path || turn.Seq != int64(i) {
			t.Errorf("turn %d metadata = %+v", i, turn)
		}
	}
	wantTypes := []string{"text", "thinking", "text", "tool_use", "tool_result"}
	for i, want := range wantTypes {
		if turns[i].Blocks[0].Type != want {
			t.Errorf("turn %d block type = %q, want %q", i, turns[i].Blocks[0].Type, want)
		}
	}
	if turns[3].Blocks[0].ToolName != "exec_command" || turns[3].Blocks[0].ToolUseID != "call-1" {
		t.Errorf("tool use = %+v", turns[3].Blocks[0])
	}
	if turns[4].Blocks[0].Text != "ok" || turns[4].Blocks[0].ToolUseID != "call-1" {
		t.Errorf("tool result = %+v", turns[4].Blocks[0])
	}
	if turns[2].Model != "gpt-5.6-sol" || turns[2].Effort != "high" || turns[2].RequestID != "request-1" || turns[2].Usage.CacheRead != 80 || turns[2].Usage.CacheCreation != 5 {
		t.Errorf("assistant request metadata = %+v", turns[2])
	}
}

func TestCodexReadRejectsMalformedRollout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&CodexSource{}).Read(Unit{Key: path}); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("Read error = %v, want line-numbered JSON error", err)
	}
}

func TestCodexCumulativeUsageCountsEveryRequestOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	lines := []string{
		`{"type":"session_meta","payload":{"id":"s","model":"fixture-model"}}`,
		`{"type":"turn_context","payload":{"turn_id":"same-user-turn"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"one"}]}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20}}}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20}}}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"two"}]}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":150,"cached_input_tokens":100,"output_tokens":40}}}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	turns, err := (&CodexSource{}).Read(Unit{Key: path})
	if err != nil {
		t.Fatal(err)
	}
	rows := Sessions(turns, nil, "")
	if len(rows) != 1 || rows[0].TotalTokens != 190 || rows[0].RequestCount != 2 || rows[0].Model != "fixture-model" {
		t.Fatalf("wrong accounting: %+v", rows)
	}
}

func TestCodexCumulativeUsageResetIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	lines := []string{
		`{"type":"session_meta","payload":{"id":"s","model":"fixture-model"}}`,
		`{"type":"turn_context","payload":{"turn_id":"first"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"one"}]}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":300,"cached_input_tokens":200,"output_tokens":30},"last_token_usage":{"input_tokens":300,"cached_input_tokens":200,"output_tokens":30}}}}`,
		`{"type":"turn_context","payload":{"turn_id":"resumed"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"two"}]}}`,
		// Resumed: the running total restarts and equals this request alone.
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20},"last_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":20}}}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":150,"cached_input_tokens":100,"output_tokens":40},"last_token_usage":{"input_tokens":50,"cached_input_tokens":20,"output_tokens":20}}}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	turns, err := (&CodexSource{}).Read(Unit{Key: path})
	if err != nil {
		t.Fatalf("a counter reset must not drop the session: %v", err)
	}
	rows := Sessions(turns, nil, "")
	// 330 before the reset, then 120 and 70 after it.
	if len(rows) != 1 || rows[0].TotalTokens != 520 || rows[0].RequestCount != 3 {
		t.Fatalf("wrong accounting across reset: %+v", rows)
	}
}
