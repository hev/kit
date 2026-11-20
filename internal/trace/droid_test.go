package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDroidReadsProjectSessionWithoutHookAuditRecords(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "-example-project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/droid.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(project, "session.jsonl")
	for path, data := range map[string][]byte{
		session: fixture, filepath.Join(project, "session.settings.json"): []byte("{}"),
		filepath.Join(root, "outside-project.jsonl"): fixture,
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := DroidSource{Root: root}
	units, err := source.Units()
	if err != nil || len(units) != 1 || units[0].Key != session || units[0].Signature == "" {
		t.Fatalf("units %+v: %v", units, err)
	}
	turns, err := source.Read(units[0])
	if err != nil || len(turns) != 4 {
		t.Fatalf("turns %+v: %v", turns, err)
	}
	if got := turns[0]; got.Harness != "droid" || got.SessionID != "00000000-0000-4000-8000-000000000001" ||
		got.Workdir != "/example/project" || got.Role != "user" || got.Seq != 1 || got.Blocks[0].Text != "List files" {
		t.Fatalf("prompt %+v", got)
	}
	if b := turns[1].Blocks; len(b) != 2 || b[0].Type != "thinking" || b[1].ToolUseID != "t1" || b[1].ToolName != "Execute" {
		t.Fatalf("tool call %+v", b)
	}
	if got := turns[2]; got.ParentUUID != "call" || got.Blocks[0].ToolUseID != "t1" || got.Blocks[0].Text != "README.md" {
		t.Fatalf("tool result %+v", got)
	}
	if b := turns[3].Blocks; len(b) != 1 || b[0].Text != "Found README.md." {
		t.Fatalf("reply %+v", b)
	}
	if err := os.WriteFile(session, []byte("{\"type\":\"session_start\",\"version\":3,\"id\":\"future\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if turns, err := source.Read(units[0]); err == nil || len(turns) != 0 {
		t.Fatalf("accepted future schema: %+v, %v", turns, err)
	}
}
