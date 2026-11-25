package evidence

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hev/kit/internal/preval"
)

func TestEvaluationEnrichmentPreservesSealedEvidence(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Put(Meta{Title: "fixture", Order: "order"}, []ItemInput{{Data: []byte("recorded contents"), Name: "proof.txt"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Path(p.ID), ManifestName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pr := preval.PullRequest{Repository: preval.Repository{Provider: preval.GitHub, Organization: "org", Repository: "123"}, ID: 1}
	link := preval.Attribution{PullRequest: pr, Provenance: preval.Provenance{OrderID: "order", AttemptID: "attempt", EvidenceIDs: []string{p.ID}}}
	obs := preval.Observation{Source: preval.Source{PullRequest: pr, Revision: "abc", EventID: "event"}, ObservedAt: time.Now(), Outcome: preval.Merged}
	e, err := preval.Project(link, obs, nil, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := e
	unrelated.Provenance.OrderID = "other"
	if err := s.SetEvaluation(p.ID, unrelated); !errors.Is(err, preval.ErrConflict) {
		t.Fatal("accepted package attached to another order")
	}
	if err := s.SetEvaluation(p.ID, e); err != nil {
		t.Fatal(err)
	}
	withEvaluation, err := s.Verify(p.ID)
	if err != nil || withEvaluation.Evaluation.ID != e.ID {
		t.Fatalf("sealed package verification or read failed: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var original, current map[string]any
	if json.Unmarshal(before, &original) != nil || json.Unmarshal(after, &current) != nil {
		t.Fatal("invalid fixture manifest")
	}
	delete(current, "evaluation")
	if !reflect.DeepEqual(original, current) {
		t.Fatal("evaluation rewrote source fields or seal")
	}
	if _, err := s.Seal(p.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("manifest was no longer sealed")
	}
}
