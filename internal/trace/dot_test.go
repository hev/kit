package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDotUnitsWalkDatePartitions(t *testing.T) {
	root := t.TempDir()
	day := filepath.Join(root, "2026-10-05")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(day, "s1.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(day, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	units, err := (&DotSource{Root: root}).Units()
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Key != path || units[0].Signature == "" {
		t.Fatalf("units = %+v", units)
	}
}

func TestDotReadKeepsTurnToolAndDurationOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s1.jsonl")
	lines := []string{
		`{"session_id":"s1","id":"e1","ts":"2026-10-05T10:00:00Z","role":"event","text":"session open"}`,
		`{"session_id":"s1","id":"u1","ts":"2026-10-05T10:00:01Z","role":"user","text":"what is on my calendar"}`,
		`{"session_id":"s1","id":"a1","ts":"2026-10-05T10:00:03Z","role":"assistant","text":"Checking.","model":"gemini-live","tool_calls":[{"id":"c1","name":"calendar","args":{"day":"today"},"result":"two meetings","is_error":false,"duration_ms":412},{"id":"c2","name":"send_text","args":{"to":"x"},"result":"denied","is_error":true,"duration_ms":7}]}`,
		`{"session_id":"s1","id":"a2","ts":"2026-10-05T10:00:05Z","role":"assistant","text":"You have two meetings."}`,
		`{"session_id":"s1","id":"empty","ts":"2026-10-05T10:00:06Z","role":"user","text":"  "}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	turns, err := (&DotSource{}).Read(Unit{Key: path})
	if err != nil {
		t.Fatal(err)
	}
	var roles []string
	for i, turn := range turns {
		roles = append(roles, turn.Role)
		if turn.Seq != int64(i) || turn.Harness != "dot" || turn.Workdir != "dot" || turn.SessionID != "s1" || turn.SourcePath != path {
			t.Fatalf("turn %d = %+v", i, turn)
		}
		if i > 0 && turn.ParentUUID != turns[i-1].TurnUUID {
			t.Fatalf("turn %d parent = %q", i, turn.ParentUUID)
		}
	}
	if got := strings.Join(roles, ","); got != "system,user,assistant,tool,assistant" {
		t.Fatalf("roles = %s", got)
	}
	use := turns[2].Blocks
	if len(use) != 3 || use[0].Type != "text" || use[1].Type != "tool_use" || use[1].ToolName != "calendar" || use[1].ToolUseID != "c1" || !strings.Contains(use[1].Text, "day: today") || use[2].ToolUseID != "c2" {
		t.Fatalf("assistant blocks = %+v", use)
	}
	res := turns[3].Blocks
	if len(res) != 2 || res[0].Type != "tool_result" || res[0].ToolUseID != "c1" || res[0].IsError ||
		!strings.Contains(res[0].Text, "two meetings") || !strings.Contains(res[0].Text, "[duration_ms: 412]") ||
		!res[1].IsError || !strings.Contains(res[1].Text, "[duration_ms: 7]") {
		t.Fatalf("result blocks = %+v", res)
	}
	if turns[2].Model != "gemini-live" {
		t.Fatalf("model = %q", turns[2].Model)
	}
}

func TestDotReadReportsBadLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&DotSource{}).Read(Unit{Key: path}); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("err = %v", err)
	}
}
