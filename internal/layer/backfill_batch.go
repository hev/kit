package layer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/trace"
)

// PatchUnknownBackfill batches sessions with no recovered commit/PR linkage.
// It writes only unknown observation fields, guarded against concurrently added
// linkage or a newer observation. Source/linkage fields are never patched.
func (c *Client) PatchUnknownBackfill(ctx context.Context, rows []trace.SessionRow) error {
	if len(rows) == 0 {
		return nil
	}
	if len(rows) > 1000 {
		return fmt.Errorf("unknown batch exceeds 1000")
	}
	if !c.Caps.Feature(FeatureConditionalWrites).usable() {
		return fmt.Errorf("conditional patches unsupported")
	}
	declared, err := c.BackfillSchema(ctx)
	if err != nil {
		return err
	}
	patches := make([]any, 0, len(rows))
	ids := make([]string, 0, len(rows))
	wants := map[string]map[string]any{}
	for _, r := range rows {
		if r.PR != "" || len(r.Commits) > 0 || r.PRState != "unknown" || r.CIState != "unknown" || r.Reverted != "unknown" {
			return fmt.Errorf("unknown batch has resolved linkage/outcome: %s", r.ID)
		}
		b, _ := json.Marshal(r.SessionOutcomes)
		var patch map[string]any
		if err = json.Unmarshal(b, &patch); err != nil {
			return err
		}
		// Dynamic attributes belong to per-commit/workflow observations, not this batch.
		for _, k := range []string{"ci_conclusions", "ci_runs", "revert_commits"} {
			var d map[string]any
			if err = json.Unmarshal(declared[k], &d); err != nil {
				return err
			}
			if d["type"] == "string" {
				b, _ := json.Marshal(patch[k])
				patch[k] = string(b)
			} else if d["type"] != "[]string" {
				return fmt.Errorf("unsupported list schema: %s", k)
			}
		}
		patch["id"] = r.ID
		patches = append(patches, patch)
		ids = append(ids, r.ID)
		wants[r.ID] = patch
	}
	var commitsDefinition map[string]any
	if err = json.Unmarshal(declared["commits"], &commitsDefinition); err != nil {
		return err
	}
	empty := any("[]")
	if commitsDefinition["type"] == "[]string" {
		empty = []string{}
	}
	condition := []any{"And", []any{
		[]any{"Or", []any{[]any{"pr", "Eq", nil}, []any{"pr", "Eq", ""}}},
		[]any{"Or", []any{[]any{"commits", "Eq", nil}, []any{"commits", "Eq", empty}}},
		[]any{"Or", []any{[]any{"outcome_checked", "Eq", nil}, []any{"outcome_checked", "Lte", map[string]any{"$ref_new": "outcome_checked"}}}},
	}}
	var out writeResponse
	if err = c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions", map[string]any{"patch_rows": patches, "patch_condition": condition, "return_affected_ids": true}, &out); err != nil {
		return err
	}
	if out.Error != "" {
		return fmt.Errorf("unknown batch: %s", out.Error)
	}
	if out.RowsAffected != len(rows) {
		return fmt.Errorf("unknown batch affected %d/%d; persisted IDs %v; checkpoint held", out.RowsAffected, len(rows), out.PatchedIDs)
	}
	if out.PatchedIDs != nil {
		seen := map[string]bool{}
		for _, id := range out.PatchedIDs {
			seen[id] = true
		}
		for _, id := range ids {
			if !seen[id] {
				return fmt.Errorf("unknown batch affected ID absent: %s", id)
			}
		}
	}
	var observed struct {
		Rows []map[string]json.RawMessage `json:"rows"`
	}
	if err = c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions/query", map[string]any{"filters": []any{"id", "In", ids}, "rank_by": []any{"id", "asc"}, "top_k": len(ids), "exclude_attributes": []string{"first_prompt", "vector"}}, &observed); err != nil {
		return err
	}
	if len(observed.Rows) != len(rows) {
		return fmt.Errorf("unknown batch readback count mismatch")
	}
	for _, raw := range observed.Rows {
		var id string
		json.Unmarshal(raw["id"], &id)
		want, ok := wants[id]
		if !ok {
			return fmt.Errorf("unexpected readback ID")
		}
		for k, v := range want {
			b, _ := json.Marshal(v)
			var a, c any
			json.Unmarshal(b, &a)
			json.Unmarshal(raw[k], &c)
			ab, _ := json.Marshal(a)
			cb, _ := json.Marshal(c)
			if string(ab) != string(cb) {
				return fmt.Errorf("unknown batch readback mismatch %s:%s", id, k)
			}
		}
	}
	return nil
}
