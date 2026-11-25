package dispatch

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/preval"
)

func orderEvaluationDocument(workspace, order, attempt string) (map[string]json.RawMessage, string, error) {
	if !filepath.IsLocal(order) || filepath.Base(order) != order {
		return nil, "", preval.ErrConflict
	}
	dir := filepath.Join(Root(), order)
	var meta Meta
	if readJSON(filepath.Join(dir, "meta.json"), &meta) != nil || meta.Forward != nil || checkRunWorkspace(workspace, meta) != nil {
		return nil, "", preval.ErrConflict
	}
	path := filepath.Join(dir, "result.json")
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
func ReadEvaluation(workspace, order, attempt string) (*preval.Evaluation, error) {
	document, _, err := orderEvaluationDocument(workspace, order, attempt)
	if err != nil || document["evaluation"] == nil {
		return nil, err
	}
	var eval *preval.Evaluation
	err = json.Unmarshal(document["evaluation"], &eval)
	return eval, err
}

// SetEvaluation patches the existing result without reserializing other fields
// through the current Result type.
func SetEvaluation(workspace string, eval preval.Evaluation) error {
	return SetEvaluationConditional(workspace, eval, "")
}

func OrderEvaluationRevision(order, attempt string, evaluation *preval.Evaluation) string {
	return preval.Revision([]string{order, attempt}, evaluation)
}

func SetEvaluationConditional(workspace string, eval preval.Evaluation, expected string) error {
	_, err := patchEvaluation(workspace, eval, expected)
	return err
}

func ClaimEvaluationLearning(workspace string, eval preval.Evaluation) (bool, error) {
	if eval.Learning.Status != "failed" {
		return false, preval.ErrConflict
	}
	return patchEvaluation(workspace, eval, "")
}

func patchEvaluation(workspace string, eval preval.Evaluation, expected string) (bool, error) {
	release, err := lockRoot()
	if err != nil {
		return false, err
	}
	defer release()
	doc, path, err := orderEvaluationDocument(workspace, eval.Provenance.OrderID, eval.Provenance.AttemptID)
	if err != nil {
		return false, err
	}
	if expected != "" {
		var previous *preval.Evaluation
		if raw := doc["evaluation"]; len(raw) > 0 && json.Unmarshal(raw, &previous) != nil {
			return false, preval.ErrConflict
		}
		if OrderEvaluationRevision(eval.Provenance.OrderID, eval.Provenance.AttemptID, previous) != expected {
			return false, preval.ErrConflict
		}
	}
	changed, err := preval.Patch(doc, eval)
	if err != nil || !changed {
		return false, err
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return false, err
	}
	err = platformops.WriteFileAtomic(path, append(body, '\n'), 0600)
	return err == nil, err
}
