package layer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hev/kit/internal/preval"
)

func TestSourceEvaluationPatchPreservesTraceAndConditionsWrite(t *testing.T) {
	s := &sessionSourceStore{rows: map[string]map[string]any{
		"session": {"id": "session", "session_id": "session", "order": "order", "parent_span_id": "attempt", "first_prompt": "original prompt", "author_id": "original author"},
	}}
	c := sourceTestClient(t, s)
	ctx := context.Background()
	current, etag, err := c.ReadEvaluation("session")
	if err != nil || current != nil || etag == "" {
		t.Fatalf("could not read historical session: %v", err)
	}
	pr := preval.PullRequest{Repository: preval.Repository{Provider: preval.GitHub, Organization: "fixture", Repository: "1"}, ID: 2}
	p := preval.Provenance{OrderID: "order", AttemptID: "attempt", TraceSessionIDs: []string{"session"}}
	link := preval.Attribution{PullRequest: pr, Provenance: p}
	observed := preval.Observation{Source: preval.Source{PullRequest: pr, Revision: "abc", EventID: "event"}, ObservedAt: time.Now(), Outcome: preval.Merged}
	evaluation, err := preval.Project(link, observed, nil, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	otherAttempt := evaluation
	otherAttempt.Provenance.AttemptID = "discarded"
	if _, err := c.SetEvaluationConditional(ctx, "session", etag, otherAttempt); !errors.Is(err, preval.ErrConflict) {
		t.Fatal("patched another attempt")
	}
	s.beforeWrite = func(store *sessionSourceStore) { store.rows["session"]["parent_span_id"] = "new-attempt" }
	if _, err := c.SetEvaluationConditional(ctx, "session", etag, evaluation); !errors.Is(err, preval.ErrConflict) {
		t.Fatal("patch won against changed source provenance")
	}
	s.mu.Lock()
	s.rows["session"]["parent_span_id"] = "attempt"
	s.mu.Unlock()
	newETag, err := c.SetEvaluationConditional(ctx, "session", etag, evaluation)
	if err != nil || newETag == etag {
		t.Fatalf("patch failed: %v", err)
	}
	if _, err := c.SetEvaluationConditional(ctx, "session", etag, evaluation); !errors.Is(err, preval.ErrConflict) {
		t.Fatal("obsolete ETag accepted")
	}
	if replay, err := c.SetEvaluationConditional(ctx, "session", newETag, evaluation); err != nil || replay != newETag {
		t.Fatalf("identical replay changed revision: %v", err)
	}
	current, readETag, err := c.ReadEvaluation("session")
	if err != nil || current == nil || current.ID != evaluation.ID || readETag != newETag {
		t.Fatalf("could not read enrichment: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if got := s.rows["session"]; got["first_prompt"] != "original prompt" || got["author_id"] != "original author" || len(s.writes) != 2 {
		t.Fatal("evaluation lost source fields or replay wrote twice")
	}
}
