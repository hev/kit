package serve

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/trace"
)

// dashboardRows reuses the existing latest identity, trace predicate and grade
// selection rules on one complete projected read. The slices belong solely to
// this request: list and Stats may consume them without a second source scan.
// Search keeps its separate ranked/phrase path.
func (s *Server) dashboardRows(r *http.Request, stats bool) ([]trace.SessionRow, string, error) {
	reader, ok := s.reader.(interface {
		ListDashboardSessionRows(bool, bool, bool) ([]trace.SessionRow, error)
	})
	if !ok {
		return s.rows(r)
	}
	q := r.URL.Query()
	if _, err := localRows(nil, q); err != nil {
		return nil, "", err
	}
	_, window, err := sessionFilter(q, nil)
	if err != nil {
		return nil, "", err
	}
	ef, err := evalFilter(q)
	if err != nil {
		return nil, "", err
	}
	// inMemoryFilter is the same predicate implementation used by value pickers.
	// Grade predicates remain on the existing latest-eval selection/join path.
	scope := r.URL.Query()
	for key := range scope {
		if key == "q" || key == "poor" || key == "role" || key == "instance" || (strings.HasPrefix(key, "mark_") && strings.HasSuffix(key, "_max")) {
			scope.Del(key)
		}
	}
	match, _, err := inMemoryFilter(scope, time.Now())
	if err != nil {
		return nil, "", err
	}
	prompts := stats || q.Get("include_stats") == "true" || q.Has("weekday") || q.Has("hour")
	all, err := reader.ListDashboardSessionRows(stats, prompts, q.Has("summary"))
	if err != nil {
		return nil, "", err
	}
	latest := latestSessions(all)
	s.allRows = latest
	excludePoor := notPoorOnly(q)
	var gradeIDs map[string]bool
	if ef != nil {
		gradeQuery := q
		if excludePoor {
			gradeQuery = map[string][]string{"poor": {"true"}}
		}
		grades, err := s.evals(gradeQuery)
		if err != nil {
			return nil, "", err
		}
		gradeIDs = map[string]bool{}
		for _, e := range grades {
			gradeIDs[e.SessionID] = true
		}
	}
	rows := make([]trace.SessionRow, 0, len(latest))
	for _, row := range latest {
		gradeOK := ef == nil || gradeIDs[row.SessionID]
		if excludePoor { // includes ungraded traces, matching rows().
			gradeOK = !gradeIDs[row.SessionID]
		}
		if gradeOK && match(row) {
			rows = append(rows, row)
		}
	}
	rows, err = localRows(rows, q)
	return rows, window, err
}

type statsModel struct {
	Tokens int64   `json:"tokens"`
	Traces int     `json:"traces"`
	Spend  float64 `json:"spend"`
}
type statsDay struct {
	Day    string                 `json:"day"`
	Spend  float64                `json:"spend"`
	Traces int                    `json:"traces"`
	Models map[string]*statsModel `json:"models"`
}
type statsProject struct {
	Name    string      `json:"name"`
	Repo    string      `json:"repo"`
	N       int         `json:"n"`
	Prompts int64       `json:"prompts"`
	Tokens  int64       `json:"tok"`
	Cost    float64     `json:"cost"`
	API     int64       `json:"api"`
	Wall    int64       `json:"wall"`
	Series  [12]float64 `json:"series"`
}

// compactStats only folds the selected rows. It makes no source-side aggregate
// or transactional source snapshot claim: Layer currently lacks complete
// latest-row-before-predicate aggregation. Day charts retain UTC start-day
// attribution; prompt heatmaps use the requested IANA timezone.
func compactStats(rows []trace.SessionRow, window string, loc *time.Location, now time.Time) map[string]any {
	days := 0
	switch window {
	case "7d":
		days = 7
	case "30d":
		days = 30
	case "90d":
		days = 90
	}
	if days == 0 {
		first := now.UnixMilli()
		for _, r := range rows {
			if r.Start != 0 && r.Start < first {
				first = r.Start
			}
		}
		days = max(1, int((now.UnixMilli()-first+86400000-1)/86400000))
	}
	var grid [7][24]int
	coverage := 0
	var cost float64
	var tokens, prompts, tools, apiMS, wallMS, cacheRead, cacheBase int64
	daily := map[string]*statsDay{}
	models := map[string]*statsModel{}
	projects := map[string]*statsProject{}
	for _, row := range rows {
		tok := row.InputTokens + row.OutputTokens + row.CacheReadTokens + row.CacheCreationTokens
		tokens += tok
		cost += row.Cost
		prompts += row.PromptCount
		tools += row.ToolCount
		apiMS += row.APIMS
		wallMS += row.WallMS
		cacheRead += row.CacheReadTokens
		cacheBase += row.InputTokens + row.CacheReadTokens + row.CacheCreationTokens
		if len(row.PromptTS) > 0 {
			coverage++
		}
		for _, ts := range row.PromptTS {
			d := time.UnixMilli(int64(ts)).In(loc)
			grid[d.Weekday()][d.Hour()]++
		}
		if models[row.Model] == nil {
			models[row.Model] = &statsModel{}
		}
		m := models[row.Model]
		m.Tokens += tok
		m.Traces++
		m.Spend += row.Cost
		name := projectName(row.RepoURL)
		if projects[name] == nil {
			projects[name] = &statsProject{Name: name, Repo: row.RepoURL}
		}
		p := projects[name]
		p.N++
		p.Prompts += row.PromptCount
		p.Tokens += tok
		p.Cost += row.Cost
		p.API += row.APIMS
		p.Wall += row.WallMS
		if p.Repo == "" {
			p.Repo = row.RepoURL
		}
		if row.Start != 0 {
			bin := int(float64(row.Start-(now.UnixMilli()-int64(days)*86400000)) / float64(int64(days)*86400000) * 12)
			p.Series[min(11, max(0, bin))] += row.Cost
			day := time.UnixMilli(row.Start).UTC().Format("2006-01-02")
			if daily[day] == nil {
				daily[day] = &statsDay{Day: day, Models: map[string]*statsModel{}}
			}
			d := daily[day]
			d.Spend += row.Cost
			d.Traces++
			if d.Models[row.Model] == nil {
				d.Models[row.Model] = &statsModel{}
			}
			m := d.Models[row.Model]
			m.Tokens += tok
			m.Traces++
			m.Spend += row.Cost
		}
	}
	ds := make([]*statsDay, 0, len(daily))
	for _, d := range daily {
		ds = append(ds, d)
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].Day < ds[j].Day })
	ps := make([]*statsProject, 0, len(projects))
	for _, p := range projects {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool {
		return ps[i].Cost > ps[j].Cost || (ps[i].Cost == ps[j].Cost && ps[i].Name < ps[j].Name)
	})
	hit := 0.0
	if cacheBase > 0 {
		hit = float64(cacheRead) / float64(cacheBase)
	}
	// Retain the Stats card denominator for legacy traces lacking prompt counts.
	perPrompt := cost / float64(max(int64(1), prompts))
	return map[string]any{
		"window": window, "days": days,
		"generated_at": now.UTC().Format(time.RFC3339Nano),
		"aggregation":  "kit_latest_rows", "timezone": loc.String(),
		"spend": cost, "tokens": tokens, "traces": len(rows),
		"api_hours": float64(apiMS) / 3600000, "wall_hours": float64(wallMS) / 3600000,
		"cache_hit": hit, "cost_per_prompt": perPrompt, "prompts": prompts, "tools": tools,
		"prompt_grid": grid, "prompt_coverage": map[string]int{"with": coverage, "total": len(rows)},
		"daily": ds, "models": models, "projects": ps, "project_count": len(ps),
	}
}
