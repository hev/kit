package trace

import (
	"encoding/json"
	"testing"
)

func TestCompactOutcomeRoundtripPreservesBoundedWire(t *testing.T) {
	detail := `{"ci_workflow_new_conclusion":"pending","commit_outcome_new_reverted":"unknown"}`
	input, _ := json.Marshal(map[string]any{"id": "s", "summary": "keep", "outcome_details": detail, "ci_workflow_existing_conclusion": "failure"})
	var row SessionRow
	if err := json.Unmarshal(input, &row); err != nil {
		t.Fatal(err)
	}
	if len(row.WorkflowAttributes) != 3 || row.WorkflowAttributes["ci_workflow_new_conclusion"] != "pending" {
		t.Fatal(row.WorkflowAttributes)
	}
	output, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var attrs map[string]any
	json.Unmarshal(output, &attrs)
	if attrs["outcome_details"] != detail || attrs["ci_workflow_existing_conclusion"] != "failure" || attrs["summary"] != "keep" {
		t.Fatal(string(output))
	}
	if attrs["ci_workflow_new_conclusion"] != nil || attrs["commit_outcome_new_reverted"] != nil {
		t.Fatal("compact fields expanded into unbounded namespace columns")
	}
	var again SessionRow
	if err = json.Unmarshal(output, &again); err != nil || len(again.WorkflowAttributes) != 3 {
		t.Fatal(err, again.WorkflowAttributes)
	}
	if json.Unmarshal([]byte(`{"id":"s","outcome_details":"{"}`), &again) == nil {
		t.Fatal("malformed outcome receipt accepted")
	}
}
