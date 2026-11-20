package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/harness"
)

func geminiFixture(t *testing.T, name, path string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestGeminiTraceFormatsAndAgyIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_AGY_DATA_DIR", "")
	gemini := &GeminiSource{Root: defaultRoot(harness.Gemini)}
	agy := &AgySource{Root: DefaultAgyRoot()}
	slash := filepath.Join(gemini.Root, "fixture-project", "chats", "session-fixture.jsonl")
	geminiFixture(t, "gemini-stream.jsonl", slash)
	project := filepath.Join(home, "workspace", "fixture")
	if err := os.WriteFile(filepath.Join(gemini.Root, "fixture-project", ".project_root"), []byte(project), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(gemini.Root, strings.Repeat("a", 64), "chats", "session-fixture.json")
	geminiFixture(t, "gemini-snapshot.json", legacy)
	geminiFixture(t, "gemini-snapshot.json", filepath.Join(agy.Root, "fixture", "chats", "session-ignored.json"))
	geminiFixture(t, "gemini-snapshot.json", filepath.Join(gemini.Root, "brain", "fixture", ".system_generated", "logs", "transcript.jsonl"))
	agyTranscript := filepath.Join(agy.Root, "brain", "fixture", ".system_generated", "logs", "transcript.jsonl")
	geminiFixture(t, "gemini-stream.jsonl", agyTranscript)
	units, err := gemini.Units()
	if err != nil || len(units) != 2 || units[0].Key != legacy || units[1].Key != slash {
		t.Fatalf("Gemini units=%+v error=%v", units, err)
	}
	unitsAgy, err := agy.Units()
	if err != nil || len(unitsAgy) != 1 || unitsAgy[0].Key != agyTranscript {
		t.Fatalf("agy units=%+v error=%v", unitsAgy, err)
	}
	for i, unit := range units {
		turns, err := gemini.Read(unit)
		if err != nil || len(turns) != 2 {
			t.Fatalf("turns=%+v error=%v", turns, err)
		}
		if turns[0].TurnUUID != "prompt" || len(turns[0].Blocks) != 1 || turns[0].Blocks[0].Text != "Check the fixture" {
			t.Fatalf("user prompt was replaced by generated context: %+v", turns[0])
		}
		got := turns[1]
		if got.Harness != "gemini" || got.Role != "assistant" || got.ParentUUID != "prompt" || len(got.Blocks) != 3 || got.Blocks[1].ToolUseID != "read" {
			t.Fatalf("assistant turn %+v", got)
		}
		if i == 0 && (got.SessionID != "fixture-legacy" || got.Workdir != "" || got.Usage.CacheRead != 8 || got.Blocks[2].IsError) {
			t.Fatalf("legacy %+v", got)
		}
		if i == 1 && (got.SessionID != "fixture-current" || got.Workdir != project || got.Summary != "Fixture check" || got.Blocks[0].Text != "Fixture is absent." || !got.Blocks[2].IsError || got.Blocks[2].Text != "missing fixture") {
			t.Fatalf("append-only %+v", got)
		}
	}
	if _, err := geminiDecode([]byte(`{"sessionId":"foreign","messages":[]}`)); err == nil {
		t.Fatal("accepted a foreign transcript")
	}
}
