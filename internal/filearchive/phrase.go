package filearchive

import (
	"fmt"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/pkg/search"
)

// PhraseHits finds chunks that contain q as contiguous text. Candidates come
// from the lexical index (every word of the phrase must be indexed there),
// then the same contiguous-text check Layer's phrase mode runs is applied.
// truncated reports that the candidate limit was reached.
func (s *Store) PhraseHits(q string, limit int, filter any) ([]layer.Hit, bool, error) {
	if search.NormalizePhrase(q) == "" || limit < 1 || limit > layer.PhraseCandidateLimit {
		return nil, false, fmt.Errorf("phrase search requires nonempty text and candidate limit 1–%d", layer.PhraseCandidateLimit)
	}
	candidates, err := s.SearchPhrasings([]string{q}, limit+1, filter)
	if err != nil {
		return nil, false, err
	}
	truncated := len(candidates) > limit
	if truncated {
		candidates = candidates[:limit]
	}
	hits := []layer.Hit{}
	for _, h := range candidates {
		if search.MatchesPhrase(h.Text, q) {
			h.Dist = 0
			hits = append(hits, h)
		}
	}
	return hits, truncated, nil
}

// PhraseEvals is empty: evaluations are stored only in Layer.
func (s *Store) PhraseEvals(string, int, any) ([]layer.Hit, bool, error) {
	return nil, false, nil
}
