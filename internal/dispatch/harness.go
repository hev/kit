package dispatch

import (
	"fmt"
	"slices"
)

// Harnesses are the agent CLIs a run, an orchestrator round or a validator
// may launch. agy is the Antigravity CLI.
var Harnesses = []string{"codex", "claude", "agy"}

// Efforts are the reasoning-effort levels an order, orchestrator or
// validation config may name.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}

// ValidHarness reports whether h names a supported harness.
func ValidHarness(h string) bool { return slices.Contains(Harnesses, h) }

// checkEffort accepts an empty effort (the harness default) or one of Efforts.
func checkEffort(e string) error {
	if e != "" && !slices.Contains(Efforts, e) {
		return fmt.Errorf("invalid effort %q: want low, medium, high, xhigh or max", e)
	}
	return nil
}

// harnessArgs builds one non-interactive harness launch: the harness's own
// print-mode flags, then --model and the effort when set, then the prompt,
// which is always the last element (wireHooks relies on that).
//
// agy's --print takes its prompt as a value, so the prompt travels as
// --print=PROMPT: a bare --print followed by flags would swallow the next
// flag as the prompt. A combined model name such as gemini-4-carbon-high is
// passed through unchanged; --effort is added only when effort is set.
func harnessArgs(harness, model, effort, prompt string) []string {
	var args []string
	switch harness {
	case "claude":
		args = []string{"-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions"}
	case "agy":
		args = []string{"--dangerously-skip-permissions", "--output-format", "stream-json"}
	default:
		args = []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		if harness == "codex" {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
		} else {
			args = append(args, "--effort", effort)
		}
	}
	if harness == "agy" {
		return append(args, "--print="+prompt)
	}
	return append(args, prompt)
}
