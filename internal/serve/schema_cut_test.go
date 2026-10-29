package serve

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDashboardMarkCeilingUsesCompactMarksWithoutDynamicQuery(t *testing.T) {
	server, fixture := dashboardFixture(t)
	fixture.mu.Lock()
	for _, row := range fixture.rows["test-evals"] {
		for key := range row {
			if strings.HasPrefix(key, "mark_") {
				t.Fatalf("eval writer created column %s", key)
			}
		}
	}
	fixture.mu.Unlock()
	result := requestJSON(t, server, "/api/sessions?window=all&mark_accuracy_max=2")
	if len(result["sessions"].([]any)) != 1 {
		t.Fatal("dashboard lost compact mark filter", result)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	for _, filter := range fixture.filters {
		wire, err := json.Marshal(filter)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(wire), "mark_accuracy") {
			t.Fatal("dynamic mark filter sent to Layer", string(wire))
		}
	}
}
