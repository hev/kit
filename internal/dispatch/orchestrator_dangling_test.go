package dispatch

import (
	"os"
	"strings"
	"testing"
)

// Regression: a scratch spec left applied after its state directory was
// removed broke ls and snapshots for every orchestrator on the host.
func TestDanglingOrchestratorSpecDoesNotBreakInventory(t *testing.T) {
	orchHome(t)
	good := applyOrch(t)
	dangling := strings.Replace(orchestratorExample, "name: release-train", "name: scratch-gone", 1)
	if _, err := ApplyOrchestrator(dangling, false); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(OrchestratorDir("scratch-gone")); err != nil {
		t.Fatal(err)
	}
	states, err := ListOrchestrators()
	if err != nil || len(states) != 2 {
		t.Fatal(states, err)
	}
	var r Response
	if err := addOrchestratorRows(&r); err != nil {
		t.Fatal(err)
	}
	seen := map[string]Row{}
	for _, row := range r.Rows {
		seen[row.Name] = row
	}
	if seen["o-"+good.Name].State == "" || seen["o-"+good.Name].State == "broken" {
		t.Fatalf("healthy orchestrator lost: %+v", r.Rows)
	}
	if b := seen["o-scratch-gone"]; b.State != "broken" || !strings.Contains(b.Error, "scratch-gone.md") {
		t.Fatalf("dangling spec not reported: %+v", b)
	}
	if _, err := PendingOrchestratorNotices(""); err != nil {
		t.Fatal(err)
	}
}
