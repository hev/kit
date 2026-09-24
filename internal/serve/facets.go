package serve

import (
	"sort"

	"github.com/hev/kit/internal/trace"
)

func facets(rows []trace.SessionRow) map[string][]string {
	sets := map[string]map[string]bool{}
	add := func(k, v string) {
		if v == "" {
			return
		}
		if sets[k] == nil {
			sets[k] = map[string]bool{}
		}
		sets[k][v] = true
	}
	for _, r := range rows {
		add("project", projectName(r.RepoURL))
		add("model", r.Model)
		add("harness", r.Harness)
		add("host", r.Host)
		for _, tool := range r.ToolNames {
			add("tool", tool)
		}
	}
	out := map[string][]string{}
	for k, set := range sets {
		for v := range set {
			out[k] = append(out[k], v)
		}
		sort.Strings(out[k])
	}
	return out
}

// archiveFacets uses rows already fetched for this request, independent of the
// selected filters, so selecting one value never hides the other choices.
func (s *Server) archiveFacets() (map[string][]string, error) {
	fc := facets(s.allRows)
	evals, err := s.evals(nil)
	if err != nil {
		return nil, err
	}
	sets := map[string]map[string]bool{"role": {}, "instance": {}, "marks": {}}
	for _, row := range evals {
		e, err := row.Eval()
		if err != nil {
			return nil, err
		}
		sets["role"][e.Role] = true
		sets["instance"][e.Instance] = true
		for name := range e.Marks {
			sets["marks"][name] = true
		}
	}
	for key, set := range sets {
		for value := range set {
			if value != "" {
				fc[key] = append(fc[key], value)
			}
		}
		sort.Strings(fc[key])
	}
	return fc, nil
}
