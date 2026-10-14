package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiNativeSessionManagerFixture(t *testing.T) {
	turns, err := (&PiSource{}).Read(Unit{Key: "testdata/pi/native-v3.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 14 {
		t.Fatalf("got %d entries", len(turns))
	}
	byID := map[string]Turn{}
	var all strings.Builder
	for _, turn := range turns {
		if turn.Harness != "pi" || turn.SessionID != "dc6a5ae0-94fa-487a-bf46-9a02a868849f" || turn.Workdir != "/synthetic/kit-pi-canary" || turn.TurnUUID == "" || turn.TS == "" {
			t.Fatalf("identity: %+v", turn)
		}
		byID[turn.TurnUUID] = turn
		for _, b := range turn.Blocks {
			all.WriteString(b.Text)
		}
	}
	if strings.Contains(all.String(), "CUSTOM_STATE_MUST_NOT_BECOME_PROMPT") {
		t.Fatal("opaque custom state leaked")
	}
	if !strings.Contains(all.String(), `"replacement":null`) || !strings.Contains(all.String(), `"firstKeptEntryId"`) {
		t.Fatal("context provenance lost")
	}
	last := turns[len(turns)-1]
	if last.Model != "synthetic-model" || byID[last.ParentUUID].Role != "user" {
		t.Fatalf("branch model inherited from wrong path: %+v", last)
	}
	rows := Sessions(turns, nil, "synthetic")
	if len(rows) != 1 {
		t.Fatalf("rows %d", len(rows))
	}
	if rows[0].InputTokens != 55 || rows[0].OutputTokens != 35 {
		t.Fatalf("usage including distinct state calls: %+v", rows[0])
	}
	for _, turn := range turns {
		for _, b := range turn.Blocks {
			if b.Type == "tool_use" && (b.ToolUseID != "native-tool-1" || b.ToolName != "bash") {
				t.Fatalf("tool provenance %+v", b)
			}
		}
	}
}
func TestPiInvalidRecordsAreAtomicAndDoNotEchoContent(t *testing.T) {
	original, err := os.ReadFile("testdata/pi/native-v3.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"\n{\"secret\":\"never-echo\"", "\n{\"type\":\"message\",\"id\":\"unknown\",\"parentId\":\"absent\",\"timestamp\":\"now\"}"} {
		path := filepath.Join(t.TempDir(), "native.jsonl")
		os.WriteFile(path, append(original, []byte(suffix)...), 0600)
		turns, err := (&PiSource{}).Read(Unit{Key: path})
		if err == nil || len(turns) != 0 || strings.Contains(err.Error(), "never-echo") {
			t.Fatalf("non-atomic parse: %v %+v", err, turns)
		}
	}
}
func TestPiUnitsAndOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HEV_PI_SESSIONS_DIR", root)
	if DefaultPiRoot() != root {
		t.Fatal("override")
	}
	os.Mkdir(filepath.Join(root, "cwd"), 0700)
	os.WriteFile(filepath.Join(root, "cwd", "native.jsonl"), []byte("test"), 0600)
	units, err := (&PiSource{Root: root}).Units()
	if err != nil || len(units) != 1 {
		t.Fatalf("%v %v", units, err)
	}
}

func TestPiChunkMetadataRetainsNativeAccountingAndTools(t *testing.T) {
	turns, err := (&PiSource{}).Read(Unit{Key: "testdata/pi/native-v3.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, turn := range turns {
		for _, chunk := range Chunks(turn) {
			if chunk.BlockType != "pi_event" {
				continue
			}
			var event struct {
				Model     string   `json:"modelUsed"`
				Provider  string   `json:"providerUsed"`
				Usage     piUsage  `json:"usage"`
				ToolCalls []string `json:"toolCallIds"`
			}
			if err := json.Unmarshal([]byte(chunk.Text), &event); err != nil {
				t.Fatal(err)
			}
			if len(event.ToolCalls) > 0 {
				found = true
				if event.ToolCalls[0] != "native-tool-1" || event.Model != "synthetic-model" || event.Provider != "synthetic" || event.Usage.Input != 11 {
					t.Fatalf("metadata %+v", event)
				}
			}
		}
	}
	if !found {
		t.Fatal("tool envelope missing")
	}
	if turnCost(Turn{Harness: "pi", Cost: 0.125, Model: "synthetic"}) != 0.125 {
		t.Fatal("native cost lost")
	}
}
