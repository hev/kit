package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopilotTextAndDiscovery(t *testing.T) {
	source := &CopilotSource{Root: "testdata/copilot"}
	units, err := source.Units()
	if err != nil || len(units) != 1 || units[0].Signature == "" {
		t.Fatalf("discovery: %v, %v", units, err)
	}
	turns, err := source.Read(units[0])
	if err != nil || len(turns) != 2 {
		t.Fatalf("read: %v, %v", turns, err)
	}
	user, assistant := turns[0], turns[1]
	if user.Role != "user" || user.SessionID != "session-1" || user.Workdir != "/example/project" ||
		user.Branch != "main" || user.Harness != "copilot" || user.Blocks[0].Text != "Hello" ||
		assistant.Role != "assistant" || assistant.ParentUUID != user.TurnUUID || assistant.Blocks[0].Text != "Hi there" {
		t.Fatalf("unexpected turns: %+v", turns)
	}
}

func TestCopilotRejectsMalformedLogWithoutLeakingContent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "session")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"events.jsonl", "other.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{synthetic-secret"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := &CopilotSource{Root: root}
	units, err := source.Units()
	if err != nil || len(units) != 1 {
		t.Fatalf("discovery: %v, %v", units, err)
	}
	if _, err := source.Read(units[0]); err == nil || strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), root) {
		t.Fatalf("parser error: %v", err)
	}
}
