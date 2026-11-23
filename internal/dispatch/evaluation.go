package dispatch

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/preval"
)

func orderEvaluationDocument(order, attempt string) (map[string]json.RawMessage, string, error) {
	if !filepath.IsLocal(order) || filepath.Base(order) != order {
		return nil, "", preval.ErrConflict
	}
	path := filepath.Join(Root(), order, "result.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var result Result
	if json.Unmarshal(body, &result) != nil || result.SpanID != attempt || result.End.IsZero() {
		return nil, "", preval.ErrConflict
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, "", err
	}
	return document, path, nil
}

// ReadEvaluation checks the active attempt identity before reading its projection.
func ReadEvaluation(order, attempt string) (*preval.Evaluation, error) {
	document, _, err := orderEvaluationDocument(order, attempt)
	if err != nil || document["evaluation"] == nil {
		return nil, err
	}
	var eval *preval.Evaluation
	err = json.Unmarshal(document["evaluation"], &eval)
	return eval, err
}

// SetEvaluation patches the existing result without reserializing other fields
// through the current Result type.
func SetEvaluation(eval preval.Evaluation) error {
	release, err := lockRoot()
	if err != nil {
		return err
	}
	defer release()
	doc, path, err := orderEvaluationDocument(eval.Provenance.OrderID, eval.Provenance.AttemptID)
	if err != nil {
		return err
	}
	changed, err := preval.Patch(doc, eval)
	if err != nil || !changed {
		return err
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(path, append(body, '\n'), 0600)
}
