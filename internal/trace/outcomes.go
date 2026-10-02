package trace

import (
	"encoding/json"
	"strings"
)

// SessionOutcomes uses explicit states: empty/unknown never implies success.
type SessionOutcomes struct {
	WorkflowAttributes map[string]string `json:"-"`
	PRState            string            `json:"pr_state"`
	PRMerged           string            `json:"pr_merged"`
	PRClosed           string            `json:"pr_closed"`
	PRURL              string            `json:"pr_url"`
	CIState            string            `json:"ci_state"`
	CIConclusions      StringList        `json:"ci_conclusions"`
	CIRuns             StringList        `json:"ci_runs"`
	Reverted           string            `json:"reverted"`
	RevertState        string            `json:"revert_state"`
	RevertCommits      StringList        `json:"revert_commits"`
	RevertUntil        int64             `json:"revert_until"`
	OutcomeChecked     int64             `json:"outcome_checked"`
}

// UnmarshalJSON retains stable flattened per-workflow fields on archive reads.
func (r *SessionRow) UnmarshalJSON(b []byte) error {
	type plain SessionRow
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*r = SessionRow(p)
	var attrs map[string]json.RawMessage
	if err := json.Unmarshal(b, &attrs); err != nil {
		return err
	}
	r.WorkflowAttributes = map[string]string{}
	for k, v := range attrs {
		if strings.HasPrefix(k, "ci_workflow_") || strings.HasPrefix(k, "commit_outcome_") {
			var value string
			if err := json.Unmarshal(v, &value); err != nil {
				return err
			}
			r.WorkflowAttributes[k] = value
		}
	}
	return nil
}

func (r SessionRow) MarshalJSON() ([]byte, error) {
	type plain SessionRow
	b, err := json.Marshal(plain(r))
	if err != nil {
		return nil, err
	}
	var attrs map[string]json.RawMessage
	if err = json.Unmarshal(b, &attrs); err != nil {
		return nil, err
	}
	for k, v := range r.WorkflowAttributes {
		attrs[k], _ = json.Marshal(v)
	}
	return json.Marshal(attrs)
}
