package filearchive

import (
	"testing"

	"github.com/hev/kit/internal/layer"
)

func TestPhraseHitsNeedContiguousText(t *testing.T) {
	s := open(t)
	if _, err := s.Write([]layer.Row{
		chunk("s1", 0, "2026-11-01T10:00:00Z", "claude_code", "Port 5432 collides when two CI jobs run"),
		chunk("s2", 0, "2026-11-01T10:00:00Z", "codex", "two jobs in CI, port 5432"),
	}); err != nil {
		t.Fatal(err)
	}
	phrase, truncated, err := s.PhraseHits("two CI jobs", 100, nil)
	if err != nil || truncated || len(phrase) != 1 || phrase[0].SessionID != "s1" {
		t.Fatalf("phrase = %+v, %v, %v", phrase, truncated, err)
	}
	if evals, _, err := s.PhraseEvals("two CI jobs", 100, nil); err != nil || len(evals) != 0 {
		t.Fatalf("evals = %+v, %v", evals, err)
	}
}
