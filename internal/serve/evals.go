package serve

import (
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
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
	for _, row := range latest {
		e, err := row.Eval()
		if err != nil {
			return nil, err
		}
		matches := true
		for _, field := range []struct{ key, value string }{{"role", e.Role}, {"instance", e.Instance}} {
			if len(q[field.key]) > 0 {
				found := false
				for _, v := range q[field.key] {
					if v == field.value {
						found = true
					}
				}
				matches = matches && found
			}
		}
		if q.Has("poor") {
			want, _ := strconv.ParseBool(q.Get("poor"))
			matches = matches && e.Poor == want
		}
		for key := range q {
			if strings.HasPrefix(key, "mark_") && strings.HasSuffix(key, "_max") {
				name := strings.TrimSuffix(strings.TrimPrefix(key, "mark_"), "_max")
				ceiling, _ := strconv.Atoi(q.Get(key))
				value, present := e.Marks[name]
				matches = matches && present && value <= ceiling
			}
		}
		if matches {
			out = append(out, row)
		}
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
	data["eval_baseline"] = s.sessionBaseline(projectName(row.RepoURL))
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
	reader   evalSource    // untimed reader: refreshes outlive the request that started them
	inflight chan struct{} // closed when the running refresh finishes
	err      error
}

// baselineWait bounds how long a transcript waits for the corpus-wide
// baseline. The scan reads every session and eval row, so on a large archive it
// can take far longer than opening one trace should.
var baselineWait = 2 * time.Second

type evalSource interface {
	evalReader
	ListSessionRows(int, any) ([]trace.SessionRow, error)
}

var errBaselinePending = errors.New("eval baseline still computing")

// sessionBaseline returns the baseline, or nil when it is not ready within
// baselineWait; the refresh keeps running and a later request gets the result.
func (s *Server) sessionBaseline(project string) *evalBaseline {
	b, err := s.evalBaselineWithin(project, baselineWait)
	if err != nil {
		return nil
	}
	return &b
}

func (s *Server) evalBaseline(project string) (evalBaseline, error) {
	return s.evalBaselineWithin(project, 0)
}

// evalBaselineWithin waits at most wait (0 means no limit) for a fresh
// baseline, else returns errBaselinePending. The mutex is never held across
// the scan, and concurrent callers share one refresh.
func (s *Server) evalBaselineWithin(project string, wait time.Duration) (evalBaseline, error) {
	c := s.baselines
	c.Lock()
	if time.Now().Before(c.expires) {
		defer c.Unlock()
		return evalBaseline{c.overall, c.projects[project]}, nil
	}
	if c.inflight == nil {
		done := make(chan struct{})
		c.inflight = done
		var src evalSource = c.reader
		if src == nil {
			src, _ = s.reader.(evalSource)
		}
		go func() {
			overall, projects, err := computeBaseline(src)
			c.Lock()
			c.err = err
			if err == nil {
				c.overall, c.projects, c.expires = overall, projects, time.Now().Add(60*time.Second)
			}
			c.inflight = nil
			c.Unlock()
			close(done)
		}()
	}
	done := c.inflight
	c.Unlock()
	var timeout <-chan time.Time
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		timeout = t.C
	}
	select {
	case <-done:
	case <-timeout:
		return evalBaseline{}, errBaselinePending
	}
	c.Lock()
	defer c.Unlock()
	if c.err != nil {
		return evalBaseline{}, c.err
	}
	return evalBaseline{c.overall, c.projects[project]}, nil
}

func computeBaseline(src evalSource) (map[string]markAverage, map[string]map[string]markAverage, error) {
	overall := map[string]markAverage{}
	projects := map[string]map[string]markAverage{}
	if src == nil {
		return overall, projects, nil
	}
	rows, err := src.ListEvalRows(nil)
	if err != nil {
		return nil, nil, err
	}
	sessions, err := src.ListSessionRows(-1, nil)
	if err != nil {
		return nil, nil, err
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
			return nil, nil, err
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
	return overall, projects, nil
}
