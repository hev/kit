package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/preval"
)

// SetEvaluation changes only metadata. It is valid on a sealed package.
func (s *Store) SetEvaluation(id string, eval preval.Evaluation) error {
	if !ValidID(id) || !slices.Contains(eval.Provenance.EvidenceIDs, id) {
		return preval.ErrConflict
	}
	release, err := s.lock()
	if err != nil {
		return err
	}
	defer release()
	pkg, err := s.Load(id)
	if err != nil {
		return err
	}
	if pkg.Order != "" && pkg.Order != eval.Provenance.OrderID ||
		pkg.Session != "" && !slices.Contains(eval.Provenance.TraceSessionIDs, pkg.Session) {
		return preval.ErrConflict
	}
	path := filepath.Join(s.dir(id), ManifestName)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	changed, err := preval.Patch(doc, eval)
	if err != nil || !changed {
		return err
	}
	data, err = json.Marshal(doc)
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(path, append(data, '\n'), 0600)
}
