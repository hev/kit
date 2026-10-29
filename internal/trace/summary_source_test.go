package trace

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSummarySourceFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/summary-source/redacted.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Harness, First, Want string
		Blocks                     []BlockRow
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if got := SummarySource(tc.First, tc.Blocks); got != tc.Want {
				t.Fatalf("source = %q; want %q", got, tc.Want)
			}
			turns := []Turn{{SessionID: "fixture", Role: "user", Harness: tc.Harness, Blocks: []Block{{Type: "text", Text: tc.First}}}}
			for _, b := range tc.Blocks {
				turns = append(turns, Turn{SessionID: "fixture", Seq: b.Seq, Role: b.Role, Harness: tc.Harness, Blocks: []Block{{Type: b.BlockType, Text: b.Text, ToolName: b.ToolName}}})
			}
			rows := Sessions(turns, nil, "")
			if len(rows) != 1 || rows[0].FirstPrompt != tc.Want || rows[0].FirstPromptShort != ShortPrompt(tc.Want) {
				t.Fatalf("projection: %+v", rows)
			}
		})
	}
}

func TestPermissionPreamblePreservesRequest(t *testing.T) {
	for _, preamble := range []string{
		"Filesystem sandboxing defines which files can be read or written.\nsandbox_mode: danger-full-access\napproval_policy: never",
		"sandbox_mode: workspace-write\napproval_policy: on-request\nnetwork access: restricted",
		"Filesystem sandboxing defines which files can be read or written.\nNetwork access is enabled.",
	} {
		const request = "Fix archive summary generation."
		if got := RequestText(preamble + "\n\n" + request); got != request {
			t.Errorf("request lost after %q: %q", preamble, got)
		}
		if got := RequestText(preamble); got != "" {
			t.Errorf("permission-only prompt retained: %q", got)
		}
	}
}
