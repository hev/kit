package preval

import (
	"encoding/json"
	"errors"
)

var ErrConflict = errors.New("evaluation attribution or revision conflict")

// Patch updates only the projection in an existing JSON object. The document
// store must hold its normal writer lock across the read and write.
func Patch(document map[string]json.RawMessage, evaluation Evaluation) (bool, error) {
	if document == nil || evaluation.Schema != Schema || evaluation.PolicyVersion != PolicyVersion ||
		!identifier.MatchString(evaluation.ID) || evaluation.ObservedAt.IsZero() {
		return false, errInvalid
	}
	next, err := json.Marshal(evaluation)
	if err != nil {
		return false, errInvalid
	}
	if raw := document["evaluation"]; len(raw) > 0 && string(raw) != "null" {
		var current Evaluation
		if json.Unmarshal(raw, &current) != nil || current.ID != evaluation.ID {
			return false, ErrConflict
		}
		if current.ObservedAt.After(evaluation.ObservedAt) {
			return false, ErrConflict
		}
		if current.ObservedAt.Equal(evaluation.ObservedAt) {
			normalized, _ := json.Marshal(current)
			if string(normalized) == string(next) {
				return false, nil
			}
			evaluationLearning := evaluation.Learning
			evaluation.Learning = current.Learning
			body, _ := json.Marshal(evaluation)
			if string(normalized) != string(body) || !learningAdvance(current.Learning.Status, evaluationLearning.Status) {
				return false, ErrConflict
			}
		}
	}
	document["evaluation"] = next
	return true, nil
}

func learningAdvance(from, to string) bool {
	return from == "pending" && (to == "failed" || to == "none" || to == "recorded") ||
		from == "failed" && (to == "none" || to == "recorded")
}
