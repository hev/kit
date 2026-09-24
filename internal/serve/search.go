package serve

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/hev/kit/internal/layer"
)

type searchReader interface {
	SearchHits(string, int, any) ([]layer.Hit, error)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) > 8192 {
		writeError(w, fmt.Errorf("q must contain 1–8192 bytes"), http.StatusBadRequest)
		return
	}
	top := 50
	if raw := r.URL.Query().Get("top"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 10000 {
			writeError(w, fmt.Errorf("top must be between 1 and 10000"), 400)
			return
		}
		top = n
	}
	rows, window, err := s.rows(r)
	if err != nil {
		writeError(w, err, 400)
		return
	}
	reader, ok := s.reader.(searchReader)
	if !ok {
		writeError(w, fmt.Errorf("search unavailable"), 503)
		return
	}
	ids := make([]string, 0, len(rows))
	byID := map[string]map[string]any{}
	joined, err := s.joinedCorpus(rows)
	if err != nil {
		writeError(w, err, 502)
		return
	}
	for _, row := range joined {
		id := row["id"].(string)
		ids = append(ids, id)
		byID[id] = row
	}
	best := map[string]layer.Hit{}
	sources := map[string]string{}
	// Keep one candidate set so Layer ranks across the entire filtered archive.
	if len(ids) > 0 {
		hits, err := reader.SearchHits(q, min(10000, max(top*4, 200)), []any{"session_id", "In", ids})
		if err != nil {
			writeError(w, err, 502)
			return
		}
		for _, hit := range hits {
			if byID[hit.SessionID] == nil {
				continue
			}
			old, ok := best[hit.SessionID]
			if !ok || hit.Dist > old.Dist {
				best[hit.SessionID] = hit
				sources[hit.SessionID] = "transcript"
			}
		}
	}
	if reader, ok := s.reader.(evalReader); ok {
		evals, err := s.evals(nil)
		if err != nil {
			writeError(w, err, 502)
			return
		}
		evalIDs := []string{}
		for _, e := range evals {
			if byID[e.SessionID] != nil {
				evalIDs = append(evalIDs, e.ID)
			}
		}
		if len(evalIDs) > 0 {
			hits, err := reader.SearchEvals(q, min(10000, max(top*4, 200)), []any{"id", "In", evalIDs})
			if err != nil {
				writeError(w, err, 502)
				return
			}
			for _, hit := range hits {
				if byID[hit.SessionID] == nil {
					continue
				}
				old, ok := best[hit.SessionID]
				if !ok || hit.Dist > old.Dist {
					best[hit.SessionID] = hit
					sources[hit.SessionID] = "eval"
				}
			}
		}
	}
	hits := make([]layer.Hit, 0, len(best))
	for _, hit := range best {
		hits = append(hits, hit)
	}
	// $dist is Layer's descending RRF score. Keep the best store-ranked hit;
	// there is no embedding, fusion, or model scoring in this server.
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Dist == hits[j].Dist {
			return hits[i].SessionID < hits[j].SessionID
		}
		return hits[i].Dist > hits[j].Dist
	})
	out := []map[string]any{}
	for _, hit := range hits[:min(top, len(hits))] {
		row := byID[hit.SessionID]
		row["snippet"] = hit.Text
		row["turn_uuid"] = hit.TurnUUID
		row["role"] = hit.Role
		row["block_type"] = hit.BlockType
		row["hit_ts"] = hit.TS
		row["tool_name"] = hit.ToolName
		row["sidechain"] = hit.Sidechain
		row["score"] = hit.Dist
		row["source"] = sources[hit.SessionID]
		out = append(out, row)
	}
	writeJSON(w, map[string]any{"window": window, "q": q, "top": top, "candidate_limit": min(10000, max(top*4, 200)), "sessions": out})
}
