package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGrokSessionUpdates(t *testing.T) {
	source := &GrokSource{Root: "testdata/grok"}
	units, err := source.Units()
	if err != nil || len(units) != 1 {
		t.Fatalf("units: %v, %v", units, err)
	}
	turns, err := source.Read(units[0])
	if err != nil || len(turns) != 4 {
		t.Fatalf("turns: %+v, %v", turns, err)
	}
	if turns[0].Role != "user" || turns[0].TS != "2026-02-02T02:40:00.123456Z" ||
		turns[0].Workdir != "/example/workspace" || turns[0].Summary != "Read example" ||
		turns[1].Blocks[0].ToolUseID != turns[2].Blocks[0].ToolUseID ||
		turns[3].Blocks[0].Text != "It contains one line." {
		t.Fatalf("decoded turns: %+v", turns)
	}

	broken := filepath.Join(t.TempDir(), "updates.jsonl")
	if err := os.WriteFile(broken, []byte("{private-path"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Read(Unit{Key: broken}); err == nil || err.Error() != "grok line 1: invalid session update JSON" {
		t.Fatalf("malformed input leaked or was accepted: %v", err)
	}
}
