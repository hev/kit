package trace

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestToolCountsTopSix(t *testing.T) {
	turns := []Turn{}
	for i := 0; i < 8; i++ {
		for j := 0; j <= i; j++ {
			turns = append(turns, Turn{SessionID: "s", Blocks: []Block{{Type: "tool_use", ToolName: fmt.Sprint(i)}}})
		}
	}
	row := Sessions(turns, nil, "")[0]
	if len(row.ToolCounts) != 6 || row.ToolCounts["7"] != 8 || row.ToolCounts["2"] != 3 || row.ToolCounts["1"] != 0 {
		t.Fatalf("counts: %v", row.ToolCounts)
	}
	for _, raw := range []string{`{"Read":3}`, `"{\"Read\":3}"`} {
		var counts ToolCounts
		if err := json.Unmarshal([]byte(raw), &counts); err != nil {
			t.Fatal(err)
		}
		if counts["Read"] != 3 {
			t.Fatal(counts)
		}
	}
}
