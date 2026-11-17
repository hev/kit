package filearchive

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/hev/kit/internal/grep"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/pkg/search"
)

// Search with no key is grep: there is no index and nothing is embedded. A
// phrasing matches a chunk that contains all of its words (case-insensitive,
// simple plurals folded), --also phrasings are OR, and hits rank by how many
// distinct words they contain, then newest first. ripgrep narrows the lines
// to read when it is on PATH; a Go walk does when it is not. A phrasing only
// finds the words it contains, so callers say so: a miss is not "nothing
// exists".

// maxPathArgs bounds how many session files a session-scoped search names
// to rg on its command line; more than that searches the directory and
// filters by session.
const maxPathArgs = 256

// Lexical reports that this store's search matches words only.
func (s *Store) Lexical() bool { return true }

// SearchCaps says what search on files can do.
func (s *Store) SearchCaps() layer.SearchCaps {
	return layer.SearchCaps{Lexical: true, Regex: true, Phrase: true, Evals: true, Engine: engine()}
}

func engine() string {
	if grep.RgPath() != "" {
		return "rg"
	}
	return "go"
}

var errPhrase = fmt.Errorf("phrase search requires nonempty text and candidate limit 1–%d", layer.PhraseCandidateLimit)

func tsMillis(ts string) int64 {
	if ts == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// Search is SearchHits with the CLI's attribute set; files keep every
// attribute, so the two are the same.
func (s *Store) Search(query string, topK int, filter any) ([]layer.Hit, error) {
	return s.SearchPhrasings([]string{query}, topK, filter)
}

// SearchHits ranks chunks for one query.
func (s *Store) SearchHits(query string, topK int, filter any) ([]layer.Hit, error) {
	return s.SearchPhrasings([]string{query}, topK, filter)
}

// SearchPhrasings finds chunks that contain every word of at least one
// phrasing. Hits carry the number of distinct words matched in Dist, highest
// first, as Layer's hybrid search puts its best score first.
func (s *Store) SearchPhrasings(phrasings []string, topK int, filter any) ([]layer.Hit, error) {
	q, err := grep.Compile(phrasings)
	if err != nil {
		return nil, nil // nothing to look for: punctuation only
	}
	return s.SearchQuery(q, topK, filter)
}

// SearchRegex finds chunks whose text matches a Go (RE2) regular
// expression, newest first.
func (s *Store) SearchRegex(pattern string, topK int, filter any) ([]layer.Hit, error) {
	q, err := grep.CompileRegex(pattern)
	if err != nil {
		return nil, err
	}
	return s.SearchQuery(q, topK, filter)
}

// SearchQuery runs a compiled query over the chunk files.
func (s *Store) SearchQuery(q *grep.Query, topK int, filter any) ([]layer.Hit, error) {
	hits, _, err := s.scan(chunkSource, q, q.Score, topK, filter)
	return hits, err
}

// PhraseHits finds chunks that contain q as contiguous text, the check
// Layer's phrase mode runs. truncated reports more matches than limit.
func (s *Store) PhraseHits(q string, limit int, filter any) ([]layer.Hit, bool, error) {
	return s.phrase(chunkSource, q, limit, filter)
}

// PhraseEvals is PhraseHits over evaluations.
func (s *Store) PhraseEvals(q string, limit int, filter any) ([]layer.Hit, bool, error) {
	return s.phrase(evalSource, q, limit, filter)
}

func (s *Store) phrase(src source, q string, limit int, filter any) ([]layer.Hit, bool, error) {
	if search.NormalizePhrase(q) == "" || limit < 1 || limit > layer.PhraseCandidateLimit {
		return nil, false, errPhrase
	}
	gq, err := grep.Compile([]string{q})
	if err != nil {
		return []layer.Hit{}, false, nil
	}
	hits, total, err := s.scan(src, gq, func(text string) (int, bool) { return 0, search.MatchesPhrase(text, q) }, limit, filter)
	if hits == nil {
		hits = []layer.Hit{}
	}
	return hits, total > limit, err
}

// SearchEvals ranks evaluations for one query, as SearchHits does chunks.
func (s *Store) SearchEvals(query string, topK int, filter any) ([]layer.Hit, error) {
	q, err := grep.Compile([]string{query})
	if err != nil {
		return nil, nil
	}
	hits, _, err := s.scan(evalSource, q, q.Score, topK, filter)
	return hits, err
}

type source int

const (
	chunkSource source = iota
	evalSource
)

// scan reads every candidate line of a source, keeps the rows score accepts
// and the filter passes, and returns the best topK by score then recency,
// with how many matched in all.
func (s *Store) scan(src source, q *grep.Query, score func(string) (int, bool), topK int, filter any) ([]layer.Hit, int, error) {
	if topK <= 0 {
		topK = 10
	}
	match, err := compileFilter(filter)
	if err != nil {
		return nil, 0, err
	}
	var allowed map[string]bool
	paths := []string{filepath.Join(s.dir, "chunks")}
	if src == evalSource {
		paths = []string{s.evalsPath()}
	} else if ids, ok := filterSessions(filter); ok {
		allowed = ids
		if len(ids) <= maxPathArgs {
			paths = paths[:0]
			for id := range ids {
				paths = append(paths, s.path("chunks", keyOf(id)))
			}
		}
	}
	minTS := filterMinTS(filter)
	type candidate struct {
		hit  layer.Hit
		rank grep.Ranked
	}
	var kept []candidate
	total := 0
	trim := func() {
		grep.Sort(kept, func(c candidate) grep.Ranked { return c.rank })
		if len(kept) > topK {
			kept = kept[:topK]
		}
	}
	var lineErr error
	var mu sync.Mutex // the callback runs on several goroutines
	err = s.lockedRead(func() error {
		return grep.Lines(context.Background(), q, paths, "*.jsonl", func(_ string, line []byte) bool {
			var hit layer.Hit
			if json.Unmarshal(line, &hit) != nil || hit.ID == "" {
				return true // a partial line can only be a file that is not ours
			}
			if allowed != nil && !allowed[hit.SessionID] {
				return true
			}
			if minTS != 0 {
				if ts := tsMillis(hit.TS); ts != 0 && ts < minTS {
					return true
				}
			}
			text := hit.Text
			if hit.ToolName != "" {
				// A search for "Bash" finds the tool calls.
				text = hit.ToolName + " " + text
			}
			n, ok := score(text)
			if !ok {
				return true
			}
			if filter != nil {
				row, err := decodeRow(line)
				if err != nil {
					mu.Lock()
					lineErr = err
					mu.Unlock()
					return false
				}
				if !match(row) {
					return true
				}
			}
			hit.Dist = float64(n)
			mu.Lock()
			defer mu.Unlock()
			total++
			kept = append(kept, candidate{hit, grep.Ranked{Score: n, TS: hit.TS, ID: hit.ID}})
			if len(kept) >= 4*topK+1024 {
				trim()
			}
			return true
		})
	})
	if err == nil {
		err = lineErr
	}
	if err != nil {
		return nil, 0, err
	}
	trim()
	hits := make([]layer.Hit, len(kept))
	for i, c := range kept {
		hits[i] = c.hit
	}
	return hits, total, nil
}

// filterMinTS finds a ["ts", "Gte"|"Gt", RFC3339] lower bound at the top of
// the filter (or in its And), so a --since search skips old chunks before
// decoding the filter's view of them. The full filter still runs on every
// hit.
func filterMinTS(raw any) int64 {
	norm, err := normalize(raw)
	if err != nil || norm == nil {
		return 0
	}
	var walk func(node any) int64
	walk = func(node any) int64 {
		f, ok := node.([]any)
		if !ok || len(f) < 2 {
			return 0
		}
		if f[0] == "And" {
			best := int64(0)
			clauses, _ := f[1].([]any)
			for _, c := range clauses {
				best = max(best, walk(c))
			}
			return best
		}
		if len(f) == 3 && f[0] == "ts" && (f[1] == "Gte" || f[1] == "Gt") {
			if s, ok := f[2].(string); ok {
				return tsMillis(s)
			}
		}
		return 0
	}
	return walk(norm)
}
