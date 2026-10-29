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
	if attrs["outcome_details"] == nil || attrs["ci_workflow_existing_conclusion"] != nil || attrs["summary"] != "keep" {
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

func TestSessionNeverSerializesBlankSummaryAsBackfilled(t *testing.T) {
	for _, summary := range []string{"", " \n ", "Fixed summary repair"} {
		b, err := json.Marshal(SessionRow{Summary: summary, SummaryBackfilled: true})
		if err != nil {
			t.Fatal(err)
		}
		var attrs map[string]any
		json.Unmarshal(b, &attrs)
		if summary != "Fixed summary repair" && attrs["summary_backfilled"] == true {
			t.Fatalf("invalid completion: %s", b)
		}
		if summary == "Fixed summary repair" && attrs["summary_backfilled"] != true {
			t.Fatalf("completion lost: %s", b)
		}
	}
}

func TestCompactOutcomesOverrideStaleAndNullLegacyColumns(t *testing.T) {
	var row SessionRow
	if err := json.Unmarshal([]byte(`{"id":"s","outcome_details":"{\"ci_workflow_existing_conclusion\":\"failure\"}","ci_workflow_existing_conclusion":"success","commit_outcome_old_reverted":null}`), &row); err != nil {
		t.Fatal(err)
	}
	if len(row.WorkflowAttributes) != 1 || row.WorkflowAttributes["ci_workflow_existing_conclusion"] != "failure" {
		t.Fatal(row.WorkflowAttributes)
	}
	wire, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var attrs map[string]any
	if err := json.Unmarshal(wire, &attrs); err != nil {
		t.Fatal(err)
	}
	if attrs["ci_workflow_existing_conclusion"] != nil || attrs["commit_outcome_old_reverted"] != nil {
		t.Fatal("legacy columns re-expanded", string(wire))
	}
}
