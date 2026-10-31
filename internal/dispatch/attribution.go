package dispatch

import (
	"os"
	"path/filepath"
	"strings"
)

// Attribution is path based and works independently of a running supervisor.
func Attribution(cwd string) (name, issue string) {
	rel, e := filepath.Rel(Root(), cwd)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || !ValidName(parts[0]) {
		return "", ""
	}
	name = parts[0]
	b, e := os.ReadFile(filepath.Join(Root(), name, "order.md"))
	if e == nil {
		if o, err := Parse(string(b)); err == nil {
			issue = o.Issue
		}
	}
	return name, issue
}
