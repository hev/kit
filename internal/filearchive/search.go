package filearchive

import (
	"encoding/json"
	"math"
	"sort"
	"sync"

	"github.com/hev/kit/internal/layer"
)

// Search with no key is lexical: Okapi BM25 per phrasing over chunk text,
// fused by reciprocal rank the way extra phrasings widen a Layer search.
// There are no embeddings, so a phrasing only finds the words it contains;
// callers should say so, so a miss is not read as "nothing exists".

const (
	bm25K1 = 1.2
	bm25B  = 0.75
	rrfK   = 60
	// maxFilterLoads bounds how many sessions a filtered search reads to find
	// rows that pass the filter.
	maxFilterLoads = 4000
	// perPhrasing caps how many ranked candidates each phrasing contributes.
	perPhrasing = 20_000
)

// Lexical reports that this store's search is lexical only.
func (s *Store) Lexical() bool { return true }

// Search is SearchHits with the CLI's attribute set; files keep every
// attribute, so the two are the same.
func (s *Store) Search(query string, topK int, filter any) ([]layer.Hit, error) {
	return s.SearchPhrasings([]string{query}, topK, filter)
}

// SearchHits ranks chunks for one query.
func (s *Store) SearchHits(query string, topK int, filter any) ([]layer.Hit, error) {
	return s.SearchPhrasings([]string{query}, topK, filter)
}

type docRef struct {
	seg int // index into the open segment list
	doc uint32
}

// SearchPhrasings ranks chunks for each phrasing and fuses the rankings.
// Hits carry the fused score in Dist, highest first, as Layer's hybrid
// search does.
func (s *Store) SearchPhrasings(phrasings []string, topK int, filter any) ([]layer.Hit, error) {
	if topK <= 0 {
		topK = 10
	}
	match, err := compileFilter(filter)
	if err != nil {
		return nil, err
	}
	var allowed map[string]bool
	if ids, ok := filterSessions(filter); ok {
		allowed = map[string]bool{}
		for id := range ids {
			allowed[keyOf(id)] = true
		}
	}
	minTS := filterMinTS(filter)
	var hits []layer.Hit
	err = s.locked(func() error {
		if err := s.refresh(); err != nil {
			return err
		}
		m, err := s.loadManifest()
		if err != nil {
			return err
		}
		var segs []*segment
		defer func() {
			for _, g := range segs {
				g.close()
			}
		}()
		var n, total float64
		for _, info := range m.Segments {
			g, err := openSegment(s.segPath(info.ID), info.ID)
			if err != nil {
				return err
			}
			segs = append(segs, g)
			n += float64(info.Docs)
			total += float64(info.TotalLen)
		}
		if n == 0 {
			return nil
		}
		avg := math.Max(total/n, 1)
		live := func(g *segment, d docRec) bool {
			key := g.keys[d.key]
			if o, ok := m.Owner[key]; !ok || o.Seg != g.id {
				return false
			}
			if allowed != nil && !allowed[key] {
				return false
			}
			return minTS == 0 || d.ts == 0 || d.ts >= minTS
		}
		fused := map[docRef]float64{}
		for _, phrasing := range phrasings {
			q := map[string]bool{}
			for _, t := range terms(phrasing) {
				q[t] = true
			}
			scores := map[docRef]float64{}
			for t := range q {
				type found struct {
					seg int
					e   dictEntry
				}
				var where []found
				df := 0.0
				for i, g := range segs {
					e, ok, err := g.lookup(t)
					if err != nil {
						return err
					}
					if ok {
						where = append(where, found{i, e})
						df += float64(e.df)
					}
				}
				if df == 0 {
					continue
				}
				idf := math.Log(1 + (n-df+0.5)/(df+0.5))
				for _, f := range where {
					g := segs[f.seg]
					ps, err := g.postings(f.e)
					if err != nil {
						return err
					}
					for _, p := range ps {
						d := g.doc(p.doc)
						if !live(g, d) {
							continue
						}
						tf := float64(p.tf)
						scores[docRef{f.seg, p.doc}] += idf * tf * (bm25K1 + 1) / (tf + bm25K1*(1-bm25B+bm25B*float64(d.len)/avg))
					}
				}
			}
			ranked := make([]docRef, 0, len(scores))
			for r := range scores {
				ranked = append(ranked, r)
			}
			sort.Slice(ranked, func(i, j int) bool {
				if scores[ranked[i]] != scores[ranked[j]] {
					return scores[ranked[i]] > scores[ranked[j]]
				}
				return lessRef(segs, ranked[i], ranked[j])
			})
			if len(ranked) > perPhrasing {
				ranked = ranked[:perPhrasing]
			}
			for rank, r := range ranked {
				fused[r] += 1.0 / float64(rrfK+rank+1)
			}
		}
		order := make([]docRef, 0, len(fused))
		for r := range fused {
			order = append(order, r)
		}
		sort.Slice(order, func(i, j int) bool {
			if fused[order[i]] != fused[order[j]] {
				return fused[order[i]] > fused[order[j]]
			}
			return lessRef(segs, order[i], order[j])
		})
		hits, err = s.resolve(segs, order, fused, match, topK)
		return err
	})
	return hits, err
}

// lessRef breaks score ties newest first, then by position, so equal scores
// always rank the same way.
func lessRef(segs []*segment, a, b docRef) bool {
	da, db := segs[a.seg].doc(a.doc), segs[b.seg].doc(b.doc)
	if da.ts != db.ts {
		return da.ts > db.ts
	}
	if a.seg != b.seg {
		return a.seg < b.seg
	}
	return a.doc < b.doc
}

// resolve walks ranked documents best first, reads their rows (a session's
// chunk file at a time, several sessions in parallel), applies the filter
// and keeps topK.
func (s *Store) resolve(segs []*segment, order []docRef, score map[docRef]float64, match compiled, topK int) ([]layer.Hit, error) {
	type loaded struct {
		rows map[string]map[string]any
		err  error
	}
	cache := map[string]*loaded{}
	var hits []layer.Hit
	loads := 0
	for start := 0; start < len(order) && len(hits) < topK && loads < maxFilterLoads; {
		// Gather the next window's unread sessions and read them together.
		end := min(len(order), start+max(64, topK*4))
		var need []string
		for _, r := range order[start:end] {
			key := segs[r.seg].keys[segs[r.seg].doc(r.doc).key]
			if cache[key] == nil {
				cache[key] = &loaded{}
				need = append(need, key)
			}
		}
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		for _, key := range need {
			wg.Add(1)
			go func(key string, l *loaded) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				lines, err := readLines(s.path("chunks", key))
				if err != nil {
					l.err = err
					return
				}
				l.rows = make(map[string]map[string]any, len(lines))
				for _, line := range lines {
					row, err := decodeRow(line)
					if err != nil {
						l.err = err
						return
					}
					if id, _ := row["id"].(string); id != "" {
						row["$raw"] = line
						l.rows[id] = row
					}
				}
			}(key, cache[key])
		}
		wg.Wait()
		loads += len(need)
		for _, r := range order[start:end] {
			if len(hits) >= topK {
				break
			}
			g := segs[r.seg]
			l := cache[g.keys[g.doc(r.doc).key]]
			if l.err != nil {
				return nil, l.err
			}
			id, err := g.chunkID(r.doc)
			if err != nil {
				return nil, err
			}
			row := l.rows[id]
			if row == nil {
				continue // rewritten since the index was refreshed
			}
			raw := row["$raw"].([]byte)
			delete(row, "$raw")
			ok := match(row)
			row["$raw"] = raw
			if !ok {
				continue
			}
			var hit layer.Hit
			if err := json.Unmarshal(raw, &hit); err != nil {
				return nil, err
			}
			hit.Dist = score[r]
			hits = append(hits, hit)
		}
		start = end
	}
	return hits, nil
}

// filterMinTS finds a ["ts", "Gte"|"Gt", RFC3339] lower bound at the top of
// the filter (or in its And), so a --since search skips old chunks without
// reading them. The full filter still runs on every hit.
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
