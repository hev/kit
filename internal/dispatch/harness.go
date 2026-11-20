package dispatch

import (
	"fmt"
	"slices"

	"github.com/hev/kit/internal/harness"
)

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
