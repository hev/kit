package serve

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func sessionFilter(q url.Values, rows []trace.SessionRow) (any, string, error) {
	base, window, err := windowFilter(q.Get("window"), time.Now())
	if err != nil {
		return nil, "", err
	}
	clauses := []any{base}
	for _, key := range []string{"model", "harness", "host"} {
		if values := q[key]; len(values) > 0 {
			clauses = append(clauses, []any{key, "In", values})
		}
	}
	if projects := q["project"]; len(projects) > 0 {
		repos := []string{}
		seen := map[string]bool{}
		for _, row := range rows {
			for _, p := range projects {
				if (p == row.RepoURL || p == projectName(row.RepoURL)) && !seen[row.RepoURL] {
					repos = append(repos, row.RepoURL)
					seen[row.RepoURL] = true
				}
			}
		}
		clauses = append(clauses, []any{"repo_url", "In", repos})
	}
	if tools := q["tool"]; len(tools) > 0 {
		clauses = append(clauses, []any{"tool_names", "ContainsAny", tools})
	}
	for _, spec := range []struct {
		key, col string
		scale    float64
	}{{"tools", "tool_count", 1}, {"tokens", "total_tokens", 1}, {"cost", "cost", 1}, {"wall", "wall_ms", 60000}} {
		bounds := map[string]float64{}
		for _, suffix := range []string{"min", "max"} {
			key := spec.key + "_" + suffix
			if raw := q.Get(key); raw != "" {
				n, err := strconv.ParseFloat(raw, 64)
				if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || ((spec.key == "tools" || spec.key == "tokens") && n != math.Trunc(n)) {
					return nil, "", fmt.Errorf("%s must be a nonnegative %s", key, map[bool]string{true: "integer", false: "number"}[spec.key == "tools" || spec.key == "tokens"])
				}
				bounds[suffix] = n
				op := "Gte"
				if suffix == "max" {
					op = "Lte"
				}
				clauses = append(clauses, []any{spec.col, op, n * spec.scale})
			}
		}
		lo, lok := bounds["min"]
		hi, hok := bounds["max"]
		if lok && hok && lo > hi {
			return nil, "", fmt.Errorf("%s_min exceeds %s_max", spec.key, spec.key)
		}
	}
	var since, until int64
	for _, key := range []string{"since", "until"} {
		if raw := q.Get(key); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				t, e := time.Parse(time.RFC3339, raw)
				if e != nil {
					t, e = time.Parse("2006-01-02", raw)
				}
				if e != nil {
					return nil, "", fmt.Errorf("%s must be RFC3339, YYYY-MM-DD, or Unix milliseconds", key)
				}
				n = t.UnixMilli()
			}
			op := "Gte"
			if key == "until" {
				op = "Lt"
				until = n
			} else {
				since = n
			}
			clauses = append(clauses, []any{"start", op, n})
		}
	}
	if since != 0 && until != 0 && since >= until {
		return nil, "", fmt.Errorf("since must precede until")
	}
	return layer.And(clauses...), window, nil
}

func evalFilter(q url.Values) (any, error) {
	clauses := []any{}
	for _, key := range []string{"role", "instance"} {
		if v := q[key]; len(v) > 0 {
			clauses = append(clauses, []any{key, "In", v})
		}
	}
	if raw := q.Get("poor"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("poor must be true or false")
		}
		clauses = append(clauses, []any{"poor", "Eq", v})
	}
	for key, values := range q {
		if strings.HasPrefix(key, "mark_") && strings.HasSuffix(key, "_max") {
			if len(values) != 1 {
				return nil, fmt.Errorf("%s must occur once", key)
			}
			n, err := strconv.Atoi(values[0])
			if err != nil {
				return nil, fmt.Errorf("%s must be an integer", key)
			}
			clauses = append(clauses, []any{strings.TrimSuffix(key, "_max"), "Lte", n})
		}
	}
	return layer.And(clauses...), nil
}

// localRows applies the predicates the server evaluates on session rows rather
// than in Layer: the prompt heatmap cell, and whether a trace has a summary.
// summary=true keeps summarized traces, the ones that read well in a demo.
func localRows(rows []trace.SessionRow, q url.Values) ([]trace.SessionRow, error) {
	rows, err := promptRows(rows, q)
	if err != nil || !q.Has("summary") {
		return rows, err
	}
	want, err := strconv.ParseBool(q.Get("summary"))
	if err != nil {
		return nil, fmt.Errorf("summary must be true or false")
	}
	out := make([]trace.SessionRow, 0, len(rows))
	for _, row := range rows {
		if (strings.TrimSpace(row.Summary) != "") == want {
			out = append(out, row)
		}
	}
	return out, nil
}
