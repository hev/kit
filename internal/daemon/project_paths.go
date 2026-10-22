package daemon

import (
	"path"
	"runtime"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// matchProjectGlob handles path components rather than treating Windows
// backslashes as glob escapes. A whole-component ** matches zero or more
// directories. This helper does not enable the currently unused capture policy.
func matchProjectGlob(pattern, project string) (bool, error) {
	pattern = expandPath(pattern)
	if runtime.GOOS == "windows" || trace.WindowsPath(pattern) || trace.WindowsPath(project) {
		pattern = strings.ToLower(strings.ReplaceAll(pattern, `\`, "/"))
		project = strings.ToLower(strings.ReplaceAll(project, `\`, "/"))
	}
	patterns := strings.Split(strings.TrimSuffix(pattern, "/"), "/")
	parts := strings.Split(strings.TrimSuffix(project, "/"), "/")
	for _, p := range patterns {
		if p != "**" {
			if _, err := path.Match(p, ""); err != nil {
				return false, err
			}
		}
	}
	// Dynamic programming bounds patterns containing repeated ** components.
	reachable := make([]bool, len(parts)+1)
	reachable[0] = true
	for _, p := range patterns {
		next := make([]bool, len(parts)+1)
		for j := 0; j <= len(parts); j++ {
			if p == "**" {
				next[j] = reachable[j] || j > 0 && next[j-1]
			} else if j > 0 && reachable[j-1] {
				next[j], _ = path.Match(p, parts[j-1])
			}
		}
		reachable = next
	}
	return reachable[len(parts)], nil
}
