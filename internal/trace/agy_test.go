package trace

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createAgySummaryDB(t *testing.T, root string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(root, "conversation_summaries.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := `
CREATE TABLE conversation_summaries (
	conversation_id TEXT PRIMARY KEY,
	title TEXT NOT NULL DEFAULT "",
	workspace_uris TEXT NOT NULL DEFAULT "",
	parent_conversation_id TEXT NOT NULL DEFAULT "",
	nesting_depth INTEGER NOT NULL DEFAULT 0,
	agent_name TEXT NOT NULL DEFAULT ""
);
CREATE TABLE conversation_parents (
	conversation_id TEXT PRIMARY KEY,
	parent_id TEXT NOT NULL
) WITHOUT ROWID;
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
}

func writeAgyTranscript(t *testing.T, root, cid, filename, content string) string {
	t.Helper()
	dir := filepath.Join(root, "brain", cid, ".system_generated", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultAgyRoot(t *testing.T) {
	t.Setenv("HEV_AGY_DATA_DIR", "/custom/agy")
	if got := DefaultAgyRoot(); got != "/custom/agy" {
		t.Fatalf("got %q, want /custom/agy", got)
	}
	t.Setenv("HEV_AGY_DATA_DIR", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".gemini", "antigravity-cli")
	if got := DefaultAgyRoot(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAgySourceMissingRoot(t *testing.T) {
	src := &AgySource{Root: filepath.Join(t.TempDir(), "missing")}
	units, err := src.Units()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(units) != 0 {
		t.Fatalf("expected 0 units, got %d", len(units))
	}
}

func TestAgyWorkspacePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`["file:///Users/hev/workspace/lyr/kit"]`, `/Users/hev/workspace/lyr/kit`},
		{`["file:///Users/hev/My%20Docs/kit"]`, `/Users/hev/My Docs/kit`},
		{`["file:///C:/Users/hev/kit"]`, `C:\Users\hev\kit`},
		{`["file://fileserver/share/repo"]`, `\\fileserver\share\repo`},
		{`[]`, ``},
		{``, ``},
		{`not-json`, ``},
	}
	for _, tc := range cases {
		if got := agyWorkspacePath(tc.in); got != tc.want {
			t.Errorf("agyWorkspacePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAgyReadSessionFull(t *testing.T) {
	root := t.TempDir()
	cid := "4566c285-654e-4c2a-b9c4-556b2056fe95"
	createAgySummaryDB(t, root,
		`INSERT INTO conversation_summaries VALUES('4566c285-654e-4c2a-b9c4-556b2056fe95','Is Kit Collecting Agy Traces?','["file:///Users/hev/workspace/lyr/kit"]','',0,'')`,
	)

	transcript := strings.Join([]string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-19T12:59:20Z","content":"<USER_REQUEST>\nis kit collecting agy traces?\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nThe current local time is: 2026-11-19T05:59:20-07:00.\n</ADDITIONAL_METADATA>\n<USER_SETTINGS_CHANGE>\nModel changed.\n</USER_SETTINGS_CHANGE>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-19T12:59:21Z","model":"gemini-4-neon-medium","input_tokens":10404,"cache_read_tokens":2000,"output_tokens":267,"thinking":"Let me check the repo and read the skill.","tool_calls":[{"name":"read_file","args":{"Path":"/Users/hev/workspace/lyr/kit/skills/hev-traces/SKILL.md"}},{"name":"run_command","args":{"CommandLine":"git grep -i agy","WaitMsBeforeAsync":5000}}]}`,
		`{"step_index":2,"source":"MODEL","type":"GENERIC","status":"ERROR","error":"file does not exist","created_at":"2026-11-19T12:59:22Z","content":"Created At: 2026-11-19T05:59:22-07:00\nCompleted At: 2026-11-19T05:59:22-07:00\nEncountered error in step execution: file does not exist"}`,
		`{"step_index":3,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-11-19T12:59:23Z","content":"Created At: 2026-11-19T05:59:23-07:00\nCompleted At: 2026-11-19T05:59:23-07:00\nCommand exited with code 1.\n\nCommand output:\n"}`,
		`{"step_index":4,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-19T12:59:24Z","model":"gemini-4-neon-medium","input_tokens":4000,"cache_read_tokens":10405,"output_tokens":120,"content":"No, kit is not collecting agy traces yet.","media":[{"mime_type":"image/png","uri":"/tmp/shot.png"}]}`,
		`{"step_index":5,"source":"MODEL","type":"PLANNER_RESPONSE","status":"RUNNING","created_at":"2026-11-19T12:59:25Z","model":"gemini-4-neon-medium","content":"streaming..."}`,
	}, "\n") + "\n"

	// Write both compact and full; Units must prefer transcript_full.jsonl.
	writeAgyTranscript(t, root, cid, "transcript.jsonl", `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"truncated"}`+"\n")
	fullPath := writeAgyTranscript(t, root, cid, "transcript_full.jsonl", transcript)

	src := &AgySource{Root: root}
	units, err := src.Units()
	if err != nil {
		t.Fatalf("Units: %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("got %d units, want 1", len(units))
	}
	if units[0].Key != fullPath {
		t.Fatalf("Key = %q, want %q", units[0].Key, fullPath)
	}
	if units[0].IsSidechain {
		t.Fatal("main conversation should not be sidechain")
	}

	turns, err := src.Read(units[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Step 5 (RUNNING) must be skipped, leaving 5 completed steps.
	if len(turns) != 5 {
		t.Fatalf("got %d turns, want 5", len(turns))
	}

	// Turn 0: USER_INPUT unwrapped cleanly.
	t0 := turns[0]
	if t0.Role != "user" || t0.Harness != "agy" || t0.SessionID != cid || t0.Workdir != "/Users/hev/workspace/lyr/kit" || t0.Summary != "Is Kit Collecting Agy Traces?" {
		t.Fatalf("unexpected turn 0 metadata: %+v", t0)
	}
	if len(t0.Blocks) != 1 || t0.Blocks[0].Text != "is kit collecting agy traces?" {
		t.Fatalf("unexpected turn 0 blocks: %+v", t0.Blocks)
	}

	// Turn 1: PLANNER_RESPONSE with thinking + 2 tool calls + usage + skill capture.
	t1 := turns[1]
	if t1.Role != "assistant" || t1.Model != "gemini-4-neon-medium" || t1.Usage.Input != 10404 || t1.Usage.CacheRead != 2000 || t1.Usage.Output != 267 {
		t.Fatalf("unexpected turn 1 metadata: %+v", t1)
	}
	if len(t1.Blocks) != 3 {
		t.Fatalf("got %d blocks on turn 1, want 3: %+v", len(t1.Blocks), t1.Blocks)
	}
	if t1.Blocks[0].Type != "thinking" {
		t.Fatalf("block 0 type = %q, want thinking", t1.Blocks[0].Type)
	}
	if t1.Blocks[1].Type != "tool_use" || t1.Blocks[1].ToolName != "read_file" {
		t.Fatalf("block 1 = %+v", t1.Blocks[1])
	}
	if len(t1.Blocks[1].SkillLoads) != 1 || t1.Blocks[1].SkillLoads[0].Name != "hev-traces" {
		t.Fatalf("expected skill load hev-traces from read_file Path, got %+v", t1.Blocks[1].SkillLoads)
	}
	call0ID := t1.Blocks[1].ToolUseID
	call1ID := t1.Blocks[2].ToolUseID

	// Turn 2: GENERIC ERROR paired with call 0 (read_file).
	t2 := turns[2]
	if t2.Role != "tool" || len(t2.Blocks) != 1 || t2.Blocks[0].Type != "tool_result" || t2.Blocks[0].ToolName != "read_file" || t2.Blocks[0].ToolUseID != call0ID || !t2.Blocks[0].IsError {
		t.Fatalf("unexpected turn 2: %+v", t2)
	}

	// Turn 3: GENERIC DONE paired with call 1 (run_command), with ExitCode=1 parsed.
	t3 := turns[3]
	if t3.Role != "tool" || len(t3.Blocks) != 1 || t3.Blocks[0].ToolName != "run_command" || t3.Blocks[0].ToolUseID != call1ID {
		t.Fatalf("unexpected turn 3: %+v", t3)
	}
	if t3.Blocks[0].ExitCode == nil || *t3.Blocks[0].ExitCode != 1 {
		t.Fatalf("expected ExitCode=1 on run_command result, got %v", t3.Blocks[0].ExitCode)
	}

	// Turn 4: final assistant text + media image reference marker.
	t4 := turns[4]
	if t4.Role != "assistant" || len(t4.Blocks) != 2 || t4.Blocks[0].Text != "No, kit is not collecting agy traces yet." {
		t.Fatalf("unexpected turn 4: %+v", t4)
	}
	if len(t4.Blocks[1].Images) != 1 || t4.Blocks[1].Images[0].Skipped == "" {
		t.Fatalf("expected skipped non-inline image marker on turn 4, got %+v", t4.Blocks[1])
	}

	// Verify session aggregation works cleanly.
	rows := Sessions(turns, nil, "laptop")
	if len(rows) != 1 {
		t.Fatalf("expected 1 session row, got %d", len(rows))
	}
	row := rows[0]
	if row.Harness != "agy" || row.Summary != "Is Kit Collecting Agy Traces?" || row.FirstPrompt != "is kit collecting agy traces?" {
		t.Fatalf("unexpected session row: %+v", row)
	}
	if row.InputTokens != 14404 || row.OutputTokens != 387 || row.CacheReadTokens != 12405 {
		t.Fatalf("unexpected token totals: in=%d out=%d cache=%d", row.InputTokens, row.OutputTokens, row.CacheReadTokens)
	}
	if len(row.Skills) != 1 || row.Skills[0] != "hev-traces" {
		t.Fatalf("unexpected session skills: %+v", row.Skills)
	}
}

func TestAgySubagentSidechainAndFallback(t *testing.T) {
	root := t.TempDir()
	parentID := "parent-conv-1"
	childID := "child-conv-2"
	createAgySummaryDB(t, root,
		`INSERT INTO conversation_summaries VALUES('child-conv-2','Research subagent','["file:///tmp/proj"]','parent-conv-1',1,'research')`,
	)
	// Only compact transcript.jsonl exists: Units must fall back to it.
	compactPath := writeAgyTranscript(t, root, childID, "transcript.jsonl",
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-19T13:00:00Z","content":"Explore the codebase"}`+"\n",
	)

	src := &AgySource{Root: root}
	units, err := src.Units()
	if err != nil {
		t.Fatalf("Units: %v", err)
	}
	if len(units) != 1 || units[0].Key != compactPath || !units[0].IsSidechain {
		t.Fatalf("unexpected units: %+v", units)
	}

	turns, err := src.Read(units[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if !turns[0].IsSidechain || turns[0].AgentID != childID || turns[0].ParentAgentID != parentID || turns[0].Workdir != "/tmp/proj" {
		t.Fatalf("unexpected sidechain turn: %+v", turns[0])
	}
}

func TestAgyWorksWithoutSummaryDB(t *testing.T) {
	root := t.TempDir()
	cid := "no-db-session"
	writeAgyTranscript(t, root, cid, "transcript_full.jsonl",
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-19T13:00:00Z","content":"hello"}`+"\n",
	)
	src := &AgySource{Root: root}
	units, err := src.Units()
	if err != nil || len(units) != 1 {
		t.Fatalf("Units: %v, %+v", err, units)
	}
	turns, err := src.Read(units[0])
	if err != nil || len(turns) != 1 || turns[0].SessionID != cid || turns[0].Harness != "agy" {
		t.Fatalf("Read: %v, %+v", err, turns)
	}
}
