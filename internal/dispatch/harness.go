package dispatch

import (
	"fmt"
	"slices"

	"github.com/hev/kit/internal/harness"
)

const DefaultOrchestratorHarness = "agy"
const DefaultOrchestratorModel = "gemini-4-neon-medium"

type FallbackTarget struct {
	Harness string `yaml:"harness" json:"harness" toml:"harness"`
	Model   string `yaml:"model,omitempty" json:"model,omitempty" toml:"model"`
	Effort  string `yaml:"effort,omitempty" json:"effort,omitempty" toml:"effort"`
}

type FallbackChain []FallbackTarget

// YAML must preserve an explicit empty chain, which disables inherited defaults.
func (f FallbackChain) IsZero() bool { return f == nil }
func (f FallbackChain) validate() error {
	for i, t := range f {
		if _, ok := harness.DispatchFor(t.Harness); !ok {
			return fmt.Errorf("fallbacks[%d]: invalid harness %q", i, t.Harness)
		}
		if err := checkEffort(t.Effort); err != nil {
			return fmt.Errorf("fallbacks[%d]: %w", i, err)
		}
	}
	return nil
}

type HarnessAttempt struct {
	FallbackTarget
	Error  string `json:"error,omitempty"`
	Status string `json:"status"`
}

// Harnesses are the agent CLIs a run, an orchestrator round or a validator
// may launch. agy is the Antigravity CLI.
var Harnesses = []string{"codex", "claude", "agy", harness.Grok}

// Efforts are the reasoning-effort levels an order, orchestrator or
// validation config may name.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}

// checkEffort accepts an empty effort (the harness default) or one of Efforts.
func checkEffort(e string) error {
	if e != "" && !slices.Contains(Efforts, e) {
		return fmt.Errorf("invalid effort %q: want low, medium, high, xhigh or max", e)
	}
	return nil
}

// harnessArgs builds one non-interactive harness launch, with the prompt last.
func harnessArgs(name, model, effort, prompt string) []string {
	descriptor, ok := harness.DispatchFor(name)
	if !ok {
		return nil
	}
	return descriptor.Dispatch.Argv(model, effort, prompt, false)
}
