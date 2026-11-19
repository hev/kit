package dispatch

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// degradeStep names what a run directory's launch is: a validator, a rework
// (any attempt after the first), or the worker.
func degradeStep(dir string, m Meta) string {
	if m.Parent != "" {
		return "validate"
	}
	var h History
	if readJSON(filepath.Join(dir, "history.json"), &h) == nil && len(h.Attempts) > 1 {
		return "rework"
	}
	return "worker"
}

// runDegradeChain is a run step's chain: from the order's harness and model
// along the configured chain, and for a validator, only models independent
// of the implementer. A config that does not load leaves the order's model
// alone, as before degrade existed.
func runDegradeChain(dir string, m Meta, cfg Config, cfgErr error) []ModelStep {
	start := ModelStep{Harness: m.Harness, Model: m.Model}
	if cfgErr != nil {
		cfg = Config{}
	}
	chain := DegradeChain(start, m.Degrade, cfg, m.Host)
	if m.Parent != "" {
		chain = independentChain(chain, m.Implementer, m.AllowSameModel)
	}
	return chain
}

// newDegradeEvent builds an event for a run step.
func newDegradeEvent(event string, m Meta, step string, from ModelStep, to *ModelStep, hit *LimitHit, reason string) DegradeEvent {
	ev := DegradeEvent{Event: event, Run: m.Name, Host: attentionHost(m.Host), Step: step, From: from, To: to, Reason: reason, Time: time.Now().UTC(), Reply: m.Reply}
	if hit != nil {
		ev.LimitType = hit.LimitType
		ev.Message = hit.Message
		if !hit.ResetsAt.IsZero() {
			t := hit.ResetsAt
			ev.ResetsAt = &t
		}
	}
	return ev
}

// readFrom returns a file's bytes from offset on.
func readFrom(path string, offset int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return nil
	}
	b, _ := io.ReadAll(f)
	return b
}

// sessionFromLogFrom is sessionFromLog for one launch's slice of the log.
func sessionFromLogFrom(path string, offset int64) string {
	for _, line := range strings.Split(string(readFrom(path, offset)), "\n") {
		var v map[string]any
		if json.Unmarshal([]byte(line), &v) != nil {
			continue
		}
		if s, ok := v["thread_id"].(string); ok {
			return s
		}
		if s, ok := v["session_id"].(string); ok {
			return s
		}
	}
	return ""
}

// implementerModels is every model a finished run ran, for its validator.
func implementerModels(m Meta, r Result) []ModelStep {
	var out []ModelStep
	for _, run := range r.Models {
		if !run.Skipped {
			out = append(out, run.ModelStep)
		}
	}
	if len(out) == 0 {
		out = append(out, ModelStep{Harness: m.Harness, Model: m.Model})
	}
	return out
}
