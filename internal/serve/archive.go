package serve

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/trace"
)

// DefaultCacheTTL is how long a fetched archive answers value pickers before
// the server asks Layer whether the sessions namespace has been written since.
const DefaultCacheTTL = time.Minute

// archiveCache keeps the newest row per session across requests, so opening a
// filter picker counts values in memory instead of rescanning the index. It is
// shared by every request, like baselines.
type archiveCache struct {
	ttl        time.Duration
	src        *Server // the long-lived server; background refreshes read through it, not a request
	mu         sync.Mutex
	rows       []trace.SessionRow
	fetched    time.Time
	watermark  string
	refreshing bool
}

type watermarker interface {
	SessionsWatermark() (string, error)
}

// pickerRows retains the columns every picker scope can consume (including
// summary and prompt-cell filters), without list previews or outcome payloads.
func (s *Server) pickerRows() ([]trace.SessionRow, error) {
	if reader, ok := s.reader.(interface {
		ListDashboardSessionRows(bool, bool, bool) ([]trace.SessionRow, error)
	}); ok {
		return reader.ListDashboardSessionRows(true, true, true)
	}
	return s.listRows(nil)
}

// latest returns the cached newest-row-per-session archive, and whether it was
// served without scanning. Only the first fill scans on the request path. Past
// the TTL the snapshot is still served while a background refresh revalidates
// it: an unchanged namespace watermark renews it, anything else rescans. A
// non-positive TTL disables the cache.
func (c *archiveCache) latest(s *Server) ([]trace.SessionRow, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rows == nil {
		mark := c.mark(s)
		all, err := s.pickerRows()
		if err != nil {
			return nil, false, err
		}
		c.rows, c.fetched, c.watermark = latestSessions(all), time.Now(), mark
		return c.rows, false, nil
	}
	if time.Since(c.fetched) >= c.ttl && !c.refreshing {
		c.refreshing = true
		go c.refresh()
	}
	return c.rows, true, nil
}

// warm fills the archive before the first picker opens.
func (c *archiveCache) warm() {
	if c.ttl > 0 {
		go func() { _, _, _ = c.latest(c.src) }()
	}
}

func (c *archiveCache) mark(s *Server) string {
	if w, ok := s.reader.(watermarker); ok {
		mark, _ := w.SessionsWatermark()
		return mark
	}
	return ""
}

// refresh revalidates the snapshot off the request path. A failed scan keeps
// the old snapshot; the next request past the TTL tries again.
func (c *archiveCache) refresh() {
	mark := c.mark(c.src)
	c.mu.Lock()
	same := mark != "" && mark == c.watermark
	if same {
		c.fetched, c.refreshing = time.Now(), false
	}
	c.mu.Unlock()
	if same {
		return
	}
	all, err := c.src.pickerRows()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshing = false
	if err == nil {
		c.rows, c.fetched, c.watermark = latestSessions(all), time.Now(), mark
	}
}

// inMemoryFilter evaluates the trace predicates of a dashboard query on
// session rows, matching what sessionFilter asks Layer for. ok is false when
// the query needs something only the store path handles: grade predicates,
// which join eval rows, or a text search.
func inMemoryFilter(q url.Values, now time.Time) (match func(trace.SessionRow) bool, ok bool, err error) {
	for key := range q {
		if key == "q" || key == "role" || key == "instance" || key == "poor" || (strings.HasPrefix(key, "mark_") && strings.HasSuffix(key, "_max")) {
			return nil, false, nil
		}
	}
	// Reuse the store compiler for validation so both paths reject the same input.
	if _, _, err := sessionFilter(q, nil); err != nil {
		return nil, true, err
	}
	var after int64
	if w := q.Get("window"); w != "all" {
		days := 30
		if w != "" {
			days, _ = strconv.Atoi(strings.TrimSuffix(w, "d"))
		}
		after = now.AddDate(0, 0, -days).UnixMilli()
	}
	since, until := int64(0), int64(0)
	if raw := q.Get("since"); raw != "" {
		since, _ = parseBound(raw)
	}
	if raw := q.Get("until"); raw != "" {
		until, _ = parseBound(raw)
	}
	in := func(key, v string) bool {
		values := q[key]
		if len(values) == 0 {
			return true
		}
		for _, x := range values {
			if x == v {
				return true
			}
		}
		return false
	}
	type bound struct {
		key    string
		column string
		scale  float64
		value  func(trace.SessionRow) float64
	}
	bounds := []bound{
		{"tools", "tool_count", 1, func(r trace.SessionRow) float64 { return float64(r.ToolCount) }},
		{"tokens", "total_tokens", 1, func(r trace.SessionRow) float64 { return float64(r.TotalTokens) }},
		{"cost", "cost", 1, func(r trace.SessionRow) float64 { return r.Cost }},
		{"wall", "wall_ms", 60000, func(r trace.SessionRow) float64 { return float64(r.WallMS) }},
	}
	return func(r trace.SessionRow) bool {
		if after != 0 && (r.MissingAttributes["end"] || r.End < after) {
			return false
		}
		if ((q.Get("since") != "" || q.Get("until") != "") && r.MissingAttributes["start"]) || (q.Get("since") != "" && r.Start < since) || (q.Get("until") != "" && r.Start >= until) {
			return false
		}
		for _, key := range []string{"project", "model", "harness", "host"} {
			col := key
			if key == "project" {
				col = "repo_url"
			}
			// Missing attributes do not match an excluded value.
			if r.MissingAttributes[col] {
				continue
			}
			for _, excluded := range q["exclude_"+key] {
				for _, value := range facetValue[key](r) {
					if excluded == value || (key == "project" && excluded == r.RepoURL) {
						return false
					}
				}
			}
		}
		for _, key := range []string{"model", "harness", "host"} {
			if len(q[key]) > 0 && r.MissingAttributes[key] {
				return false
			}
		}
		if !in("model", r.Model) || !in("harness", r.Harness) || !in("host", r.Host) {
			return false
		}
		if projects := q["project"]; len(projects) > 0 && (r.MissingAttributes["repo_url"] || (!in("project", r.RepoURL) && !in("project", projectName(r.RepoURL)))) {
			return false
		}
		if skills := q["skill"]; len(skills) > 0 {
			hit := false
			for _, skill := range r.Skills {
				if in("skill", skill) {
					hit = true
					break
				}
			}
			if !hit {
				return false
			}
		}
		if tools := q["tool"]; len(tools) > 0 {
			hit := false
			for _, t := range r.ToolNames {
				if in("tool", t) {
					hit = true
					break
				}
			}
			if !hit {
				return false
			}
		}
		for _, b := range bounds {
			if (q.Get(b.key+"_min") != "" || q.Get(b.key+"_max") != "") && r.MissingAttributes[b.column] {
				return false
			}
			v := b.value(r)
			if raw := q.Get(b.key + "_min"); raw != "" {
				if n, _ := strconv.ParseFloat(raw, 64); v < n*b.scale {
					return false
				}
			}
			if raw := q.Get(b.key + "_max"); raw != "" {
				if n, _ := strconv.ParseFloat(raw, 64); v > n*b.scale {
					return false
				}
			}
		}
		return true
	}, true, nil
}

// parseBound reads a since/until value: Unix milliseconds, RFC3339 or a date.
func parseBound(raw string) (int64, error) {
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t, err = time.Parse("2006-01-02", raw)
	}
	if err != nil {
		return 0, fmt.Errorf("must be RFC3339, YYYY-MM-DD, or Unix milliseconds")
	}
	return t.UnixMilli(), nil
}
