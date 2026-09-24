package serve

import (
	"testing"
	"time"
)

func TestEvalBaselineLatestAndCache(t *testing.T) {
	s, f := dashboardFixture(t)
	data := requestJSON(t, s, "/api/session/trace-0")
	baseline := data["eval_baseline"].(map[string]any)
	all := baseline["overall"].(map[string]any)
	accuracy := all["accuracy"].(map[string]any)
	if accuracy["avg"] != 3.5 || accuracy["n"] != float64(2) {
		t.Fatalf("latest averages: %v", all)
	}
	if all["clarity"].(map[string]any)["n"] != float64(1) {
		t.Fatal("missing marks counted")
	}
	project := baseline["project"].(map[string]any)
	if project["accuracy"].(map[string]any)["avg"] != float64(2) {
		t.Fatalf("project: %v", project)
	}
	f.mu.Lock()
	for _, r := range f.rows["test-evals"] {
		if r["session_id"] == "trace-0" {
			r["marks"] = `{"accuracy":4}`
		}
	}
	f.mu.Unlock()
	// A new timed request must reuse the server cache, while its ordinary
	// evaluation snapshot must see the updated row.
	next := requestJSON(t, s, "/api/session/trace-0")
	if next["eval_baseline"].(map[string]any)["overall"].(map[string]any)["accuracy"].(map[string]any)["avg"] != 3.5 {
		t.Fatal("request-local timing reset the shared baseline")
	}
	if next["eval"].(map[string]any)["marks"].(map[string]any)["accuracy"] != float64(4) {
		t.Fatal("request reused stale evaluation data")
	}
	if next["timing"].(map[string]any)["queries"] != float64(3) {
		t.Fatalf("cached request repeated baseline queries: %v", next["timing"])
	}
	cached, err := s.evalBaseline("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if cached.Overall["accuracy"].Avg != 3.5 {
		t.Fatal("cache did not retain snapshot")
	}
	s.baselines.expires = time.Now().Add(-time.Second)
	fresh, err := s.evalBaseline("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Overall["accuracy"].Avg != 4.5 || fresh.Project["accuracy"].Avg != 4 {
		t.Fatalf("refresh: %+v", fresh)
	}
}
