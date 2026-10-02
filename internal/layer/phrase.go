package layer

import (
	"fmt"

	"github.com/hev/kit/pkg/search"
)

// PhraseCandidateLimit bounds rows inspected per namespace, independently of
// the dashboard's final session limit. One extra row detects truncation.
const PhraseCandidateLimit = 10000

// PhraseHits scans filtered transcript rows by ID, then checks contiguous text.
// It needs ordered_scan, never a full-text or embedding capability.
func (c *Client) PhraseHits(q string, limit int, filter any) ([]Hit, bool, error) {
	return c.phraseHits(q, limit, filter, []string{"text", "session_id", "turn_uuid", "ts", "role", "block_type", "tool_name", "is_sidechain"})
}

// PhraseEvals checks evaluator matched text (the text attribute, not summary).
func (c *Client) PhraseEvals(q string, limit int, filter any) ([]Hit, bool, error) {
	cl := *c
	cl.Namespace += "-evals"
	hits, truncated, err := cl.phraseHits(q, limit, filter, []string{"text", "session_id", "ts", "role"})
	if isNamespaceMissing(err) {
		return nil, false, nil
	}
	return hits, truncated, err
}

func (c *Client) phraseHits(q string, limit int, filter any, attrs []string) ([]Hit, bool, error) {
	if c.Caps.Feature(FeatureOrderedScan) != Supported {
		return nil, false, fmt.Errorf("phrase search requires supported ordered_scan")
	}
	if search.NormalizePhrase(q) == "" || limit < 1 || limit > PhraseCandidateLimit {
		return nil, false, fmt.Errorf("phrase search requires nonempty text and candidate limit 1–%d", PhraseCandidateLimit)
	}
	hits := []Hit{}
	cursor, inspected := "", 0
	for {
		pageSize := min(1000, limit-inspected+1)
		f := filter
		if cursor != "" {
			f = And(f, []any{"id", "Gt", cursor})
		}
		body := map[string]any{"rank_by": []any{"id", "asc"}, "top_k": pageSize, "include_attributes": attrs}
		if f != nil {
			body["filters"] = f
		}
		var out struct {
			Rows  []Hit  `json:"rows"`
			Error string `json:"error"`
		}
		if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"/query", body, &out); err != nil {
			return nil, false, err
		}
		if out.Error != "" {
			return nil, false, fmt.Errorf("layer phrase query: %s", out.Error)
		}
		for _, hit := range out.Rows {
			if inspected == limit {
				return hits, true, nil
			}
			if hit.ID <= cursor {
				return nil, false, fmt.Errorf("phrase pagination did not advance")
			}
			cursor = hit.ID
			inspected++
			if search.MatchesPhrase(hit.Text, q) {
				hit.Dist = 0 // An ordered scan supplies no relevance score.
				hits = append(hits, hit)
			}
		}
		if len(out.Rows) < pageSize {
			return hits, false, nil
		}
	}
}
