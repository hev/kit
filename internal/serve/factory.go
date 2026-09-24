package serve

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/cost"
)

type FactoryConfig struct {
	Rows, Prices, Subscriptions, Outcomes, Team string
	Repos                                       []string
}

func (s *Server) WithFactory(c FactoryConfig) *Server { s.factoryConfig = &c; return s }

type costTotal struct {
	Sessions    int     `json:"sessions"`
	Workers     int     `json:"worker_sessions"`
	Beats       int     `json:"beats"`
	Tokens      int64   `json:"tokens"`
	API         float64 `json:"api_usd"`
	Sub         float64 `json:"sub_usd"`
	Incomplete  int     `json:"incomplete_sessions"`
	Unpriced    int     `json:"unpriced"`
	Unallocated int     `json:"unallocated"`
}

func (t *costTotal) add(r cost.Row) {
	t.Sessions++
	if len(r.Errors) > 0 || r.AccountingError != "" || r.Model == "" || r.Tokens == 0 || (r.Role == "worker" && r.Issue == "") || (r.Role != "other" && r.Instance == "") {
		t.Incomplete++
	}
	if r.Role == "worker" {
		t.Workers++
	}
	t.Tokens += r.Tokens
	if r.API == nil {
		t.Unpriced++
	} else {
		t.API += *r.API
	}
	if r.Sub == nil {
		t.Unallocated++
	} else {
		t.Sub += *r.Sub
	}
}

type costLine struct {
	costTotal
	Instance string                `json:"instance"`
	Roles    map[string]*costTotal `json:"roles"`
}

func factoryMatch(r cost.Row, q url.Values) bool {
	for key, value := range map[string]string{"instance": r.Instance, "role": r.Role, "model": r.Model, "harness": r.Harness, "project": r.Repo} {
		if values := q[key]; len(values) > 0 {
			found := false
			for _, v := range values {
				if v == value || key == "project" && v == projectName(value) {
					found = true
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}
func factoryWindow(q url.Values, now time.Time) (int64, int64, error) {
	raw := q.Get("window")
	if raw == "" {
		raw = "7d"
	}
	if _, _, err := windowFilter(raw, now); err != nil {
		return 0, 0, err
	}
	start := int64(0)
	end := now.UnixMilli()
	if raw != "all" {
		days, _ := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		start = now.AddDate(0, 0, -days).UnixMilli()
	}
	for _, key := range []string{"since", "until"} {
		if raw := q.Get(key); raw != "" {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				t, e := time.Parse(time.RFC3339, raw)
				if e != nil {
					t, e = time.Parse("2006-01-02", raw)
				}
				if e != nil {
					return 0, 0, e
				}
				v = t.UnixMilli()
			}
			if key == "since" {
				start = v
			} else {
				end = v
			}
		}
	}
	if start >= end {
		return 0, 0, fmt.Errorf("since must precede until")
	}
	return start, end, nil
}
func (s *Server) factory(w http.ResponseWriter, r *http.Request) {
	c := s.factoryConfig
	if c == nil || c.Rows == "" {
		writeError(w, fmt.Errorf("factory accounting is not configured"), 503)
		return
	}
	now := time.Now()
	q := r.URL.Query()
	for key := range q {
		switch key {
		case "window", "since", "until", "instance", "role", "model", "harness", "project", "timezone", "session":
		default:
			writeError(w, fmt.Errorf("%s is not available for accounting rows; clear that trace filter", key), 400)
			return
		}
	}
	start, end, err := factoryWindow(q, now)
	if err != nil {
		writeError(w, err, 400)
		return
	}
	cfg, err := cost.LoadConfig(c.Prices, c.Subscriptions)
	if err != nil {
		writeError(w, err, 503)
		return
	}
	rows, err := cost.ReadRows(c.Rows)
	if err != nil {
		writeError(w, err, 503)
		return
	}
	// Allocate against the entire input BEFORE applying dashboard predicates.
	if err = cfg.Price(rows); err != nil {
		writeError(w, err, 503)
		return
	}
	lines := map[string]*costLine{}
	issues := map[string]*costLine{}
	total := costTotal{}
	arbitrage := costTotal{}
	selected := []cost.Row{}
	for _, row := range rows {
		if factoryMatch(row, q) && cfg.InCurrentWeek(row, now) {
			arbitrage.add(row)
		}
		if row.End < start || row.Start >= end || !factoryMatch(row, q) {
			continue
		}
		row.Requests = nil // Pricing detail is not needed by the dashboard payload.
		selected = append(selected, row)
		total.add(row)
		name := row.Instance
		if name == "" {
			name = "other"
		}
		line := lines[name]
		if line == nil {
			line = &costLine{Instance: name, Roles: map[string]*costTotal{}}
			lines[name] = line
		}
		line.add(row)
		role := line.Roles[row.Role]
		if role == nil {
			role = &costTotal{}
			line.Roles[row.Role] = role
		}
		role.add(row)
	}
	// Issue cost-to-date deliberately ignores the selected time window.
	for _, row := range rows {
		if row.Issue == "" || !factoryMatch(row, q) {
			continue
		}
		issue := issues[row.Issue]
		if issue == nil {
			issue = &costLine{Roles: map[string]*costTotal{}}
			issues[row.Issue] = issue
		}
		issue.add(row)
		role := issue.Roles[row.Role]
		if role == nil {
			role = &costTotal{}
			issue.Roles[row.Role] = role
		}
		role.add(row)
	}
	beats, beatErr := cost.ReadBeats(filepath.Join(filepath.Dir(c.Rows), "beats.jsonl"))
	beatError := ""
	if beatErr != nil {
		beatError = beatErr.Error()
	}
	for _, beat := range beats {
		if beat.Timestamp < start || beat.Timestamp >= end {
			continue
		}
		row := cost.Row{Instance: beat.Instance, Role: "gaffer"}
		if !factoryMatch(row, q) {
			continue
		}
		line := lines[beat.Instance]
		if line == nil {
			line = &costLine{Instance: beat.Instance, Roles: map[string]*costTotal{}}
			lines[beat.Instance] = line
		}
		line.Beats++
		role := line.Roles["gaffer"]
		if role == nil {
			role = &costTotal{}
			line.Roles["gaffer"] = role
		}
		role.Beats++
		total.Beats++
	}
	outcomes := cost.Outcomes{Issues: []cost.Outcome{}}
	outcomeError := ""
	if c.Outcomes == "" {
		outcomeError = "gaffer-provided outcome cache missing"
	} else {
		outcomes, err = cost.ReadOutcomes(c.Outcomes, c.Team, c.Repos, now)
		if err != nil {
			outcomeError = err.Error()
			outcomes = cost.Outcomes{Issues: []cost.Outcome{}}
		}
	}
	merged := map[string]bool{}
	deploys := map[string]bool{}
	// A verified deploy need not reference an issue. Include all scoped deploys
	// when no session attribution filter is selected; project can match directly.
	if len(q["instance"])+len(q["role"])+len(q["model"])+len(q["harness"]) == 0 {
		for _, p := range outcomes.PRs {
			if p.MergedAt >= start && p.MergedAt < end && factoryMatch(cost.Row{Repo: p.Repo}, url.Values{"project": q["project"]}) {
				merged[p.URL] = true
			}
		}
		for _, d := range outcomes.Deploys {
			if d.LandedAt >= start && d.LandedAt < end && factoryMatch(cost.Row{Repo: d.Repo}, url.Values{"project": q["project"]}) {
				deploys[d.Repo+"/"+d.ID] = true
			}
		}
	}
	done := 0
	for _, i := range outcomes.Issues {
		if _, ok := issues[i.Issue]; !ok && (len(q["instance"])+len(q["role"])+len(q["model"])+len(q["harness"])+len(q["project"]) > 0) {
			continue
		}
		if i.DoneAt >= start && i.DoneAt < end {
			done++
		}
		for _, p := range i.PRs {
			if p.MergedAt >= start && p.MergedAt < end && factoryMatch(cost.Row{Repo: p.Repo}, url.Values{"project": q["project"]}) {
				merged[p.URL] = true
			}
		}
		for _, d := range i.Deploys {
			if d.LandedAt >= start && d.LandedAt < end && factoryMatch(cost.Row{Repo: d.Repo}, url.Values{"project": q["project"]}) {
				deploys[d.Repo+"/"+d.ID] = true
			}
		}
	}
	var perMerge *float64
	if len(merged) > 0 && total.Unpriced == 0 {
		v := total.API / float64(len(merged))
		perMerge = &v
	}
	limits := cfg.Limits(rows, now)
	usageError := ""
	if err := cost.ApplyClaudeUsage(filepath.Join(filepath.Dir(c.Rows), "claude-usage.json"), limits, now); err != nil && !os.IsNotExist(err) {
		usageError = err.Error()
	}
	coverageStart := now.UnixMilli()
	for _, row := range rows {
		if row.Start < coverageStart {
			coverageStart = row.Start
		}
	}
	facets := map[string][]string{}
	for _, row := range rows {
		for key, value := range map[string]string{"instance": row.Instance, "role": row.Role, "model": row.Model, "harness": row.Harness, "project": row.Repo} {
			if value == "" {
				continue
			}
			found := false
			for _, v := range facets[key] {
				if v == value {
					found = true
				}
			}
			if !found {
				facets[key] = append(facets[key], value)
			}
		}
	}
	info, _ := os.Stat(c.Rows)
	var modified int64
	if info != nil {
		modified = info.ModTime().UnixMilli()
	}
	writeJSON(w, map[string]any{"start": start, "end": end, "lines": lines, "issues": issues, "sessions": selected, "incomplete_sessions": total.Incomplete, "total": total, "arbitrage": arbitrage, "plans": cfg.Plans, "limits": limits, "usage_error": usageError, "facets": facets,
		"outcomes": outcomes, "outcome_error": outcomeError, "outcomes_stale": outcomes.RefreshedAt < now.Add(-time.Hour).UnixMilli(),
		"shipping":   map[string]any{"issues_done": done, "prs_merged": len(merged), "deploys_landed": len(deploys), "api_per_merge": perMerge},
		"beat_error": beatError, "allocation_note": "Subscription dollars are a token-share allocation of fixed weekly spend, not measured usage. Filters do not change the weekly denominator.", "input_updated_at": modified, "coverage_start": coverageStart})
}

// ResolveFactoryPaths provides predictable paths without introducing identity or
// provisioning dependencies. Every input is an ordinary operator-written file.
func ResolveFactoryPaths(dir string) FactoryConfig {
	return FactoryConfig{Rows: filepath.Join(dir, "sessions.jsonl"), Prices: filepath.Join(dir, "prices.toml"), Subscriptions: filepath.Join(dir, "subscriptions.toml"), Outcomes: filepath.Join(dir, "outcomes.json")}
}
