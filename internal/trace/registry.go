package trace

import (
	"slices"

	"github.com/hev/kit/internal/harness"
)

// sourceFactories binds registry identities to native trace adapters. Keeping
// these factories here lets the registry stay independent of the trace model.
var sourceFactories = map[string]func(string) Source{
	harness.Claude:   func(root string) Source { return &ClaudeSource{Root: root} },
	harness.Codex:    func(root string) Source { return &CodexSource{Root: root} },
	harness.Dot:      func(root string) Source { return &DotSource{Root: root} },
	harness.Pi:       func(root string) Source { return &PiSource{Root: root} },
	harness.OpenCode: func(root string) Source { return &OpenCodeSource{Root: root} },
	harness.Copilot:  func(root string) Source { return &CopilotSource{Root: root} },
	harness.Agy:      func(root string) Source { return &AgySource{Root: root} },
}

func defaultRoot(name string) string {
	d, _ := harness.Lookup(name)
	return d.Trace.Root()
}

// Sources returns the registered native sources in discovery order. An
// explicitly supplied root, even an empty one, is passed to its adapter.
func Sources(roots map[string]string) []Source {
	return sources(harness.All(), roots)
}

// PreviewSources preserves the skill preview's historical block order.
func PreviewSources(roots map[string]string) []Source {
	descriptors := harness.All()
	slices.SortStableFunc(descriptors, func(a, b harness.Descriptor) int {
		tail := func(d harness.Descriptor) int {
			if d.Trace != nil && d.Trace.PreviewTail {
				return 1
			}
			return 0
		}
		return tail(a) - tail(b)
	})
	return sources(descriptors, roots)
}

func sources(descriptors []harness.Descriptor, roots map[string]string) []Source {
	var sources []Source
	for _, d := range descriptors {
		factory := sourceFactories[d.Name]
		if d.Trace == nil || factory == nil {
			continue
		}
		root, supplied := roots[d.Name]
		if !supplied {
			root = d.Trace.Root()
		}
		sources = append(sources, factory(root))
	}
	return sources
}

// SourceInfo describes a native source. Other implementations of Source can
// participate in indexing without claiming a native discovery identity.
func SourceInfo(src Source) (root, name string) {
	if scoped, ok := src.(interface{ nativeSource() (string, string) }); ok {
		root, canonical := scoped.nativeSource()
		d, _ := harness.Lookup(canonical)
		return root, d.TraceName
	}
	return "", ""
}

func (s *ClaudeSource) nativeSource() (string, string)   { return s.Root, harness.Claude }
func (s *CodexSource) nativeSource() (string, string)    { return s.Root, harness.Codex }
func (s *DotSource) nativeSource() (string, string)      { return s.Root, harness.Dot }
func (s *PiSource) nativeSource() (string, string)       { return s.Root, harness.Pi }
func (s *OpenCodeSource) nativeSource() (string, string) { return s.Root, harness.OpenCode }
func (s *CopilotSource) nativeSource() (string, string)  { return s.Root, harness.Copilot }
func (s *AgySource) nativeSource() (string, string)      { return s.Root, harness.Agy }
