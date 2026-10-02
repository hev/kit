package layer

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/trace"
)

func outcomeSchema(arrays bool) map[string]any {
	s := scalarSchema("pr_state", "pr_merged", "pr_closed", "pr_url", "ci_state", "reverted", "revert_state", "revert_until", "outcome_checked")
	for _, k := range []string{"revert_until", "outcome_checked"} {
		s[k] = map[string]any{"type": "int", "filterable": true}
	}
	for _, k := range []string{"ci_conclusions", "ci_runs", "revert_commits"} {
		t := "string"
		if arrays {
			t = "[]string"
		}
		s[k] = map[string]any{"type": t, "filterable": true}
	}
	return s
}

// OutcomePage is a bounded ID cursor scan, including unchanged transcripts.
func (c *Client) OutcomePage(after string, since int64, limit int) ([]trace.SessionRow, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("outcome page limit must be 1..1000")
	}
	f := any([]any{"end", "Gte", since})
	if after != "" {
		f = And(f, []any{"id", "Gt", after})
	}
	var out struct {
		Rows  []trace.SessionRow `json:"rows"`
		Error string             `json:"error"`
	}
	err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-sessions/query", map[string]any{"rank_by": []any{"id", "asc"}, "top_k": limit, "filters": f, "exclude_attributes": []string{"first_prompt", "vector"}}, &out)
	if isNamespaceMissing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("outcome query: %s", out.Error)
	}
	return out.Rows, nil
}

// PatchOutcomes touches only enrichment, never transcript or summaries.
func (c *Client) PatchOutcomes(row trace.SessionRow) error {
	if err := c.SummariesServed(); err != nil {
		return err
	}
	b, err := json.Marshal(row.SessionOutcomes)
	if err != nil {
		return err
	}
	var patch map[string]any
	if err = json.Unmarshal(b, &patch); err != nil {
		return err
	}
	patch["id"] = row.ID
	patch["pr"] = row.PR
	s := outcomeSchema(c.Caps.Arrays())
	s["pr"] = map[string]any{"type": "string", "filterable": true}
	if !c.Caps.Arrays() {
		for _, k := range []string{"ci_conclusions", "ci_runs", "revert_commits"} {
			b, _ := json.Marshal(patch[k])
			patch[k] = string(b)
		}
	}
	var out writeResponse
	if err = c.do("POST", "/v2/namespaces/"+c.Namespace+"-sessions", map[string]any{"patch_rows": []any{patch}, "schema": s}, &out); err != nil {
		return err
	}
	if out.Error != "" {
		return fmt.Errorf("outcome patch: %s", out.Error)
	}
	return nil
}
