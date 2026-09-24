package serve

import (
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

type evalReader interface {
	ListEvalRows(any) ([]layer.EvalRow, error)
	SearchEvals(string, int, any) ([]layer.Hit, error)
}

func latestEvals(rows []layer.EvalRow) []layer.EvalRow {
	byID := map[string]layer.EvalRow{}
	for _, r := range rows {
		old, ok := byID[r.SessionID]
		a, _ := time.Parse(time.RFC3339Nano, r.TS)
		b, _ := time.Parse(time.RFC3339Nano, old.TS)
		if !ok || a.After(b) || (a.Equal(b) && r.ID > old.ID) {
			byID[r.SessionID] = r
		}
	}
	out := []layer.EvalRow{}
	for _, r := range byID {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionID < out[j].SessionID })
	return out
}
func (s *Server) evals(q url.Values) ([]layer.EvalRow, error) {
	f, err := evalFilter(q)
	if err != nil {
		return nil, err
	}
	reader, ok := s.reader.(evalReader)
	if !ok {
		return nil, nil
	}
	list := reader.ListEvalRows
	if slim, ok := s.reader.(interface {
		ListEvalMarks(any) ([]layer.EvalRow, error)
	}); ok {
		list = slim.ListEvalMarks
	}
	rows := s.evalRows
	if !s.evalLoaded {
		rows, err = list(nil)
		if err == nil {
			s.evalRows = rows
			s.evalLoaded = true
		}
	}
	if err != nil {
		return nil, err
	}
	latest := latestEvals(rows)
	if f == nil {
		return latest, nil
	}
	out := []layer.EvalRow{}
	for start := 0; start < len(latest); start += 1000 {
		ids := []string{}
		for _, r := range latest[start:min(start+1000, len(latest))] {
			ids = append(ids, r.ID)
		}
		batch, err := list(layer.And(f, []any{"id", "In", ids}))
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
	}
	return out, nil
}
func (s *Server) joinedCorpus(rows []trace.SessionRow) ([]map[string]any, error) {
	evals, err := s.evals(nil)
	if err != nil {
		return nil, err
	}
	byID := map[string]*trace.Eval{}
	for _, row := range evals {
		e, err := row.Eval()
		if err != nil {
			return nil, err
		}
		byID[row.SessionID] = e
	}
	out := corpus(rows)
	for _, row := range out {
		row["eval"] = nil
		if e := byID[row["id"].(string)]; e != nil {
			row["eval"] = map[string]any{"marks": e.Marks, "poor": e.Poor, "role": e.Role, "instance": e.Instance}
		}
	}
	return out, nil
}
func (s *Server) sessionData(row trace.SessionRow, blocks []trace.BlockRow) (map[string]any, error) {
	data := buildSession(row, blocks)
	baseline, err := s.evalBaseline(projectName(row.RepoURL))
	if err != nil {
		return nil, err
	}
	data["eval_baseline"] = baseline
	reader, ok := s.reader.(evalReader)
	if !ok {
		return data, nil
	}
	rows, err := reader.ListEvalRows([]any{"session_id", "Eq", row.SessionID})
	if err != nil {
		return nil, err
	}
	rows = latestEvals(rows)
	if len(rows) > 0 {
		e, err := rows[0].Eval()
		if err != nil {
			return nil, err
		}
		data["eval"] = e
	}
	return data, nil
}

// Each mark has its own denominator: older graders may emit different marks.
type markAverage struct {
	Avg float64 `json:"avg"`
	N   int     `json:"n"`
}
type evalBaseline struct {
	Overall map[string]markAverage `json:"overall"`
	Project map[string]markAverage `json:"project"`
}
type baselineCache struct {
	sync.Mutex
	expires  time.Time
	overall  map[string]markAverage
	projects map[string]map[string]markAverage
}

func (s *Server) evalBaseline(project string) (evalBaseline, error) {
	c := s.baselines
	c.Lock()
	defer c.Unlock()
	if time.Now().Before(c.expires) {
		return evalBaseline{c.overall, c.projects[project]}, nil
	}
	overall := map[string]markAverage{}
	projects := map[string]map[string]markAverage{}
	reader, ok := s.reader.(evalReader)
	if ok {
		rows, err := reader.ListEvalRows(nil)
		if err != nil {
			return evalBaseline{}, err
		}
		sessions, err := s.reader.ListSessionRows(-1, nil)
		if err != nil {
			return evalBaseline{}, err
		}
		byID := map[string]string{}
		for _, row := range latestSessions(sessions) {
			byID[row.SessionID] = projectName(row.RepoURL)
		}
		add := func(dst map[string]markAverage, name string, value int) {
			a := dst[name]
			a.Avg = (a.Avg*float64(a.N) + float64(value)) / float64(a.N+1)
			a.N++
			dst[name] = a
		}
		for _, row := range latestEvals(rows) {
			e, err := row.Eval()
			if err != nil {
				return evalBaseline{}, err
			}
			p, found := byID[row.SessionID]
			if found && projects[p] == nil {
				projects[p] = map[string]markAverage{}
			}
			for name, value := range e.Marks {
				add(overall, name, value)
				if found {
					add(projects[p], name, value)
				}
			}
		}
	}
	c.overall = overall
	c.projects = projects
	c.expires = time.Now().Add(60 * time.Second)
	return evalBaseline{overall, projects[project]}, nil
}
