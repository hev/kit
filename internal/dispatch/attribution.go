package dispatch

import (
	"os"
	"path/filepath"
	"strings"
)

func attributionName(cwd string) string {
	rel, e := filepath.Rel(Root(), cwd)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || !ValidName(parts[0]) {
		return ""
	}
	return parts[0]
}

// Attribution is path based and works independently of a running supervisor.
func Attribution(cwd string) (name, issue string) {
	name = attributionName(cwd)
	if name == "" {
		return "", ""
	}
	orderName := name
	var m Meta
	if readJSON(filepath.Join(Root(), name, "meta.json"), &m) == nil && m.Parent != "" {
		name = m.Parent
	}
	b, e := os.ReadFile(filepath.Join(Root(), orderName, "order.md"))
	if e == nil {
		if o, err := Parse(string(b)); err == nil {
			issue = o.Issue
		}
	}
	return name, issue
}

// TraceAttribution joins a worker session to its run's trace: the trace ID
// and the span of the attempt (or validation run) that launched it. The
// attempt comes from the result that recorded the session; a session no
// result names falls back to the run's current attempt. Untraced runs and
// paths outside dispatch return empty strings.
func TraceAttribution(cwd, session string) (traceID, parentSpan string) {
	name := attributionName(cwd)
	if name == "" {
		return "", ""
	}
	dir := filepath.Join(Root(), name)
	var m Meta
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil || !ValidTraceID(m.TraceID) {
		return "", ""
	}
	for _, r := range runResults(dir) {
		if session != "" && r.Session == session && r.TraceID == m.TraceID && ValidSpanID(r.SpanID) {
			return m.TraceID, r.SpanID
		}
	}
	if ValidSpanID(m.SpanID) {
		return m.TraceID, m.SpanID
	}
	return m.TraceID, ""
}

// runResults reads the current result and every archived attempt's result.
func runResults(dir string) []Result {
	paths, _ := filepath.Glob(filepath.Join(dir, "result.json.*"))
	paths = append(paths, filepath.Join(dir, "result.json"))
	var out []Result
	for _, p := range paths {
		if strings.HasSuffix(p, ".tmp") {
			continue
		}
		var r Result
		if readJSON(p, &r) == nil {
			out = append(out, r)
		}
	}
	return out
}
