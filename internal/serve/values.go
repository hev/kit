package serve

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/hev/kit/internal/trace"
)

// ui holds the layer-ui components the page imports; scripts/import-layer-ui.py
// stages them from a verified layer-ui runtime build.
//
//go:embed ui
var ui embed.FS

func uiHandler() http.Handler {
	sub, err := fs.Sub(ui, "ui")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/ui/", http.FileServerFS(sub))
}

// facetValue names what a session row contributes to each value facet.
var facetValue = map[string]func(trace.SessionRow) []string{
	"project": func(r trace.SessionRow) []string { return []string{projectName(r.RepoURL)} },
	"model":   func(r trace.SessionRow) []string { return []string{r.Model} },
	"harness": func(r trace.SessionRow) []string { return []string{r.Harness} },
	"host":    func(r trace.SessionRow) []string { return []string{r.Host} },
	"tool":    func(r trace.SessionRow) []string { return r.ToolNames },
}

// values counts one facet's values across the traces every other filter keeps.
// The facet's own selection is left out, so choosing a value never hides its
// siblings, and a count is how many traces choosing that value would show.
func (s *Server) values(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	facet := q.Get("facet")
	value, ok := facetValue[facet]
	if !ok {
		writeError(w, fmt.Errorf("facet must be project, model, tool, harness, or host"), http.StatusBadRequest)
		return
	}
	q.Del("facet")
	q.Del(facet)
	q.Del("q")
	rows, cached, err := s.valueRows(r, q)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	counts := map[string]int{}
	for _, row := range rows {
		seen := map[string]bool{}
		for _, v := range value(row) {
			if v != "" && !seen[v] {
				seen[v] = true
				counts[v]++
			}
		}
	}
	out := make([]map[string]any, 0, len(counts))
	for v, n := range counts {
		out = append(out, map[string]any{"v": v, "n": n})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i]["n"].(int), out[j]["n"].(int)
		return a > b || (a == b && out[i]["v"].(string) < out[j]["v"].(string))
	})
	writeJSON(w, map[string]any{"values": out, "total": len(out), "truncated": false, "cached": cached})
}

// valueRows are the traces a picker counts over. Trace predicates run in
// memory on the shared archive; grade and text predicates go to the store.
func (s *Server) valueRows(r *http.Request, q url.Values) ([]trace.SessionRow, bool, error) {
	match, ok, err := inMemoryFilter(q, time.Now())
	if err != nil {
		return nil, false, err
	}
	if ok && s.archive != nil && s.archive.ttl > 0 {
		all, cached, err := s.archive.latest(s)
		if err != nil {
			return nil, false, err
		}
		rows := make([]trace.SessionRow, 0, len(all))
		for _, row := range all {
			if match(row) {
				rows = append(rows, row)
			}
		}
		rows, err = localRows(rows, q)
		return rows, cached, err
	}
	scoped := r.Clone(r.Context())
	scoped.URL.RawQuery = q.Encode()
	rows, _, err := s.rows(scoped)
	return rows, false, err
}
