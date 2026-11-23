package preval

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

var ErrConflict = errors.New("evaluation attribution or revision conflict")

// Patch updates only the projection in an existing JSON object. The document
// store must hold its normal writer lock across the read and write.
func Patch(document map[string]json.RawMessage, evaluation Evaluation) (bool, error) {
	if document == nil || !evaluation.Valid() {
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

// Revision binds a strong precondition to the source identity and projection.
func Revision(identity []string, current *Evaluation) string {
	body, _ := json.Marshal(struct {
		Identity []string
		Current  *Evaluation
	}{identity, current})
	return fmt.Sprintf(`"%x"`, sha256.Sum256(body))
}

func traceFields(document map[string]json.RawMessage) []string {
	keys := []string{"id", "session_id", "order", "parent_span_id"}
	values := make([]string, len(keys))
	for i, key := range keys {
		_ = json.Unmarshal(document[key], &values[i])
	}
	return values
}

func TraceRevision(document map[string]json.RawMessage) (*Evaluation, string, error) {
	var evaluation *Evaluation
	if raw := document["evaluation"]; len(raw) > 0 && json.Unmarshal(raw, &evaluation) != nil {
		return nil, "", errInvalid
	}
	return evaluation, Revision(traceFields(document), evaluation), nil
}

func TraceMatches(document map[string]json.RawMessage, id string, p Provenance) bool {
	fields := traceFields(document)
	return fields[0] == id && slices.Contains(p.TraceSessionIDs, fields[1]) &&
		fields[2] == p.OrderID && fields[3] == p.AttemptID
}
