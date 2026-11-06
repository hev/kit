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
