package layer

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/hev/kit/internal/preval"
)

func (c *Client) ReadEvaluation(id string) (*preval.Evaluation, string, error) {
	doc, err := c.readSessionSource(id)
	if err != nil {
		return nil, "", err
	}
	if doc == nil {
		return nil, "", os.ErrNotExist
	}
	return preval.TraceRevision(doc)
}

// SetEvaluationConditional patches one existing session without reindexing
// the document or letting an older revision win a concurrent write.
func (c *Client) SetEvaluationConditional(ctx context.Context, id, expected string, eval preval.Evaluation) (string, error) {
	if !c.Caps.Feature(FeatureConditionalWrites).usable() || !c.Caps.Feature(FeaturePatchRows).usable() {
		return "", errors.New("trace evaluation requires conditional patches")
	}
	doc, err := c.WithContext(ctx).readSessionSource(id)
	if err != nil {
		return "", err
	}
	if doc == nil {
		return "", os.ErrNotExist
	}
	_, revision, err := preval.TraceRevision(doc)
	if err != nil {
		return "", err
	}
	if expected != "" && expected != revision || !preval.TraceMatches(doc, id, eval.Provenance) {
		return "", preval.ErrConflict
	}
	conditions := make([]any, 0, 5)
	for _, field := range []string{"id", "session_id", "order", "parent_span_id", "evaluation"} {
		var value any
		if raw := doc[field]; len(raw) != 0 && json.Unmarshal(raw, &value) != nil {
			return "", preval.ErrConflict
		}
		conditions = append(conditions, []any{field, "Eq", value})
	}
	changed, err := preval.Patch(doc, eval)
	if err != nil || !changed {
		return revision, err
	}
	body := map[string]any{
		"patch_rows":          []any{map[string]any{"id": id, "evaluation": string(doc["evaluation"])}},
		"patch_condition":     []any{"And", conditions},
		"schema":              map[string]any{"evaluation": map[string]any{"type": "string", "filterable": false}},
		"return_affected_ids": true,
	}
	var out writeResponse
	if err := c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions", body, &out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", errors.New("trace evaluation patch failed")
	}
	if out.RowsAffected != 1 || len(out.PatchedIDs) != 0 && (len(out.PatchedIDs) != 1 || out.PatchedIDs[0] != id) {
		return "", preval.ErrConflict
	}
	_, revision, err = preval.TraceRevision(doc)
	return revision, err
}
