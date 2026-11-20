package trace

import (
	"slices"
	"testing"

	"github.com/hev/kit/internal/harness"
)

func TestRegisteredSourceOrder(t *testing.T) {
	roots := map[string]string{
		harness.Claude:   "claude root",
		harness.Codex:    "",
		harness.Pi:       "pi root",
		harness.OpenCode: "opencode root",
		harness.Dot:      "dot root",
		harness.Agy:      "agy root",
		harness.Cursor:   "cursor root",
	}
	for _, tc := range []struct {
		name    string
		sources []Source
		want    []string
	}{
		{"discovery", Sources(roots), []string{"claude_code", "codex", "dot", "pi", "opencode", "agy", "cursor"}},
		{"preview", PreviewSources(roots), []string{"claude_code", "codex", "pi", "opencode", "cursor", "dot", "agy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var names []string
			for _, src := range tc.sources {
				root, name := SourceInfo(src)
				d, ok := harness.Lookup(name)
				if want, supplied := roots[d.Name]; !ok || !supplied || root != want {
					t.Errorf("source %q root = %q, want %q", name, root, want)
				}
				names = append(names, name)
			}
			if !slices.Equal(names, tc.want) {
				t.Errorf("sources = %v, want %v", names, tc.want)
			}
		})
	}
}
