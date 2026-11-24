package attention

import "slices"

// Subscription filters lifecycle notices. Empty source and host lists match all.
type Subscription struct {
	Events  []string `yaml:"events" json:"events" toml:"events"`
	Sources []string `yaml:"sources,omitempty" json:"sources,omitempty" toml:"sources"`
	Hosts   []string `yaml:"hosts,omitempty" json:"hosts,omitempty" toml:"hosts"`
}

// ValidSource reports whether kit emits notices for source.
func ValidSource(source string) bool {
	switch source {
	case SourceRun, SourceOrchestrator, SourceFollow, SourceSwarm, SourceSteer, SourceCron, SourceService:
		return true
	default:
		return false
	}
}

func (s Subscription) Wants(event EventSummary) bool {
	return slices.Contains(s.Events, event.Event) &&
		(len(s.Sources) == 0 || slices.Contains(s.Sources, event.Source))
}

// EventSummary is the only notice data orchestrator subscriptions receive.
// In particular, it carries no free-form text, links or requester data.
type EventSummary struct {
	Event  string `json:"event"`
	Source string `json:"source"`
	Host   string `json:"host"`
	Name   string `json:"name"`
	State  string `json:"state,omitempty"`
}

// OrchestratorTarget is an explicit forwarding route on an emitting host.
type OrchestratorTarget struct {
	Name string `toml:"name"`
	Host string `toml:"host"`
	Subscription
}
