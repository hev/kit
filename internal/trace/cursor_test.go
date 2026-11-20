package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCursorTranscriptDiscoveryAndRead(t *testing.T) {
	root := t.TempDir()
	fixture, err := os.ReadFile("testdata/cursor/example.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"project/agent-transcripts/flat.jsonl", "project/agent-transcripts/nested/nested.jsonl", "project/other/ignored.jsonl"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, fixture, 0600); err != nil {
			t.Fatal(err)
		}
	}
	src := &CursorSource{Root: root}
	units, err := src.Units()
	if err != nil || len(units) != 2 {
		t.Fatalf("units = %+v, %v", units, err)
	}
	for i, session := range []string{"flat", "nested"} {
		turns, err := src.Read(units[i])
		if err != nil || len(turns) != 3 {
			t.Fatalf("turns = %+v, %v", turns, err)
		}
		if turns[0].SessionID != session || turns[0].Harness != "cursor" ||
			turns[0].Role != "user" || turns[1].Role != "assistant" ||
			turns[2].ParentUUID != turns[1].TurnUUID || len(turns[1].Blocks) != 2 ||
			turns[1].Blocks[1].Type != "tool_use" || turns[1].Blocks[1].ToolName != "Read" {
			t.Fatalf("incorrect transcript mapping: %+v", turns)
		}
	}
}
