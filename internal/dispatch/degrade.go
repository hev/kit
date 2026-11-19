package dispatch

import (
	"fmt"
	"strings"
	"time"
)

// Degrade (docs/dispatch.md#degrade): when a harness exits because its plan
// is out of usage, the same step relaunches on the next model in a chain
// instead of failing. The chain stays in the harness first (Fable 5.2, Opus
// 5.5, Sonnet 5.5) and then crosses over to Codex on the host's configured
// Codex model. It never reaches past models the host can already run.

// ModelStep is one link of a degrade chain: a harness and a model. An empty
// Model is the harness's own configured default (for Codex, the host's
// ~/.codex/config.toml model).
type ModelStep struct {
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
}

// String is the config spelling: harness/model, or the bare harness when the
// model is the harness's default.
func (s ModelStep) String() string {
	if s.Model == "" {
		return s.Harness
	}
	return s.Harness + "/" + s.Model
}

// Short is the ls spelling: the model without its vendor prefix
// (claude-opus-5-5 is opus-5-5), or the harness for its default model.
func (s ModelStep) Short() string {
	if s.Model == "" {
		return s.Harness
	}
	return strings.TrimPrefix(s.Model, "claude-")
}

// same mirrors CheckValidator's rule for "the same harness and model": the
// same named model, or the same harness when neither names one.
func (s ModelStep) same(o ModelStep) bool {
	return s.Model == o.Model && (s.Model != "" || s.Harness == o.Harness)
}

// ParseModelStep reads a chain entry: "claude/MODEL", "codex/MODEL", a bare
// harness ("codex", "claude") for its default model, or a bare claude-* model.
func ParseModelStep(v string) (ModelStep, error) {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	harness, model, ok := strings.Cut(v, "/")
	if !ok {
		switch {
		case v == "codex" || v == "claude":
			harness, model = v, ""
		case strings.HasPrefix(v, "claude-"):
			harness, model = "claude", v
		default:
			return ModelStep{}, fmt.Errorf("degrade: %q: use harness/model, a bare harness, or a claude-* model", v)
		}
	}
	if harness != "codex" && harness != "claude" {
		return ModelStep{}, fmt.Errorf("degrade: %q: unknown harness %q", v, harness)
	}
	if ok && (model == "" || strings.ContainsAny(model, " \t/")) {
		return ModelStep{}, fmt.Errorf("degrade: %q: invalid model", v)
	}
	return ModelStep{Harness: harness, Model: model}, nil
}

// DefaultDegradeChain is the chain when no config or order names one.
var DefaultDegradeChain = []string{"claude/claude-fable-5-2", "claude/claude-opus-5-5", "claude/claude-sonnet-5-5", "codex"}

// DegradeConfig is the [degrade] table, globally and per host
// ([hosts.NAME.degrade]). A host's fields replace the global ones they set.
type DegradeConfig struct {
	// Chain is the ordered model list (ParseModelStep entries).
	Chain []string `toml:"chain"`
	// Off disables degrade: every launch runs on its order's model only.
	Off bool `toml:"off"`
	// OnDegrade is a command (argv) run with a DegradeEvent as JSON on
	// standard input for every degrade and exhausted event.
	OnDegrade []string `toml:"on_degrade"`
	// HookTimeout bounds OnDegrade (a Go duration; default 10s).
	HookTimeout string `toml:"hook_timeout"`
	// Observations is a factory directory (sessions.jsonl, prices.toml,
	// subscriptions.toml, claude-usage.json) whose fresh plan observations
	// let a launch skip a model already known to be exhausted.
	Observations string `toml:"observations"`
}

// degradeFor merges the host's [hosts.NAME.degrade] over the global table.
func (c Config) degradeFor(host string) DegradeConfig {
	d := c.Degrade
	h, ok := c.Hosts[host]
	if !ok || h.Degrade == nil {
		return d
	}
	if len(h.Degrade.Chain) > 0 {
		d.Chain = h.Degrade.Chain
	}
	if h.Degrade.Off {
		d.Off = true
	}
	if len(h.Degrade.OnDegrade) > 0 {
		d.OnDegrade = h.Degrade.OnDegrade
	}
	if h.Degrade.HookTimeout != "" {
		d.HookTimeout = h.Degrade.HookTimeout
	}
	if h.Degrade.Observations != "" {
		d.Observations = h.Degrade.Observations
	}
	return d
}

// checkDegrade validates a [degrade] table at config load.
func checkDegrade(where string, d *DegradeConfig) error {
	if d == nil {
		return nil
	}
	for _, e := range d.Chain {
		if _, err := ParseModelStep(e); err != nil {
			return fmt.Errorf("%s.chain: %w", where, err)
		}
	}
	if d.HookTimeout != "" {
		if t, err := time.ParseDuration(d.HookTimeout); err != nil || t <= 0 {
			return fmt.Errorf("%s.hook_timeout: invalid duration %q", where, d.HookTimeout)
		}
	}
	return nil
}

// DegradeOff is the order value that pins a run to its one model.
const DegradeOff = "off"

// parseDegrade normalizes an order's degrade: value. It accepts off, on
// (the configured chain), or a chain as a flow list [a, b] or one entry, and
// returns "", "off" or the entries joined by commas.
func parseDegrade(v string) (string, error) {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, "#"); i >= 0 && (!strings.HasPrefix(v, "[") || i > strings.Index(v, "]")) {
		v = strings.TrimSpace(v[:i])
	}
	switch v {
	case "off", "false", "none":
		return DegradeOff, nil
	case "on", "true", "default":
		return "", nil
	}
	if strings.HasPrefix(v, "[") {
		if !strings.HasSuffix(v, "]") {
			return "", fmt.Errorf("degrade: expected off or [harness/model, ...]")
		}
		v = v[1 : len(v)-1]
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		s, err := ParseModelStep(item)
		if err != nil {
			return "", err
		}
		out = append(out, s.String())
	}
	if len(out) == 0 {
		return "", fmt.Errorf("degrade: empty chain; use degrade: off to pin one model")
	}
	return strings.Join(out, ","), nil
}

// DegradeChain is the ordered list of models a step may run on. It starts at
// start (the order's harness and model, or a validator's or orchestrator
// round's). The list is the order's override, else the host's chain, else
// the global chain, else DefaultDegradeChain. When start is in the list the
// chain continues from there, so an order on Opus degrades to Sonnet, then
// Codex; when it is not, the whole list follows start. degrade: off, or
// [degrade] off, is start alone.
func DegradeChain(start ModelStep, orderDegrade string, cfg Config, host string) []ModelStep {
	d := cfg.degradeFor(host)
	if orderDegrade != "" {
		if v, err := parseDegrade(orderDegrade); err == nil {
			orderDegrade = v
		}
	}
	if orderDegrade == DegradeOff || (d.Off && orderDegrade == "") {
		return []ModelStep{start}
	}
	list := d.Chain
	if orderDegrade != "" {
		list = strings.Split(orderDegrade, ",")
	}
	if len(list) == 0 {
		list = DefaultDegradeChain
	}
	var steps []ModelStep
	for _, e := range list {
		if s, err := ParseModelStep(e); err == nil {
			steps = append(steps, s)
		}
	}
	for i, s := range steps {
		if s.Harness == start.Harness && s.Model == start.Model {
			steps = steps[i+1:]
			break
		}
	}
	chain := []ModelStep{start}
	for _, s := range steps {
		dup := false
		for _, c := range chain {
			if c.Harness == s.Harness && c.Model == s.Model {
				dup = true
				break
			}
		}
		if !dup {
			chain = append(chain, s)
		}
	}
	return chain
}

// independentChain drops every step that would make a validator the same
// harness and model as the implementer it checks, matching CheckValidator.
// With allow (--allow-same-model) nothing is dropped.
func independentChain(chain []ModelStep, implementer []ModelStep, allow bool) []ModelStep {
	if allow || len(implementer) == 0 {
		return chain
	}
	var out []ModelStep
	for _, s := range chain {
		same := false
		for _, i := range implementer {
			if s.same(i) {
				same = true
				break
			}
		}
		if !same {
			out = append(out, s)
		}
	}
	return out
}

// ModelRun records one launch of a step: the model that ran, why it was
// chosen, and how it ended. Result.Models lists every one.
type ModelRun struct {
	ModelStep
	// Reason is "order" for the first model, else why the step degraded
	// to this one.
	Reason   string    `json:"reason"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end,omitempty"`
	ExitCode int       `json:"exit_code"`
	// Limit is set when this launch ended on a plan limit.
	Limit *LimitHit `json:"limit,omitempty"`
	// Skipped means a fresh observation said this model was exhausted, so
	// it was never launched.
	Skipped bool `json:"skipped,omitempty"`
}

// DegradeEvent is the on_degrade hook payload and the peers notice's source.
// Event is "degrade" or "exhausted"; an exhausted event is urgent.
type DegradeEvent struct {
	Event string `json:"event"`
	Run   string `json:"run"`
	Host  string `json:"host"`
	// Step is worker, rework, validate or orchestrator.
	Step      string     `json:"step"`
	From      ModelStep  `json:"from"`
	To        *ModelStep `json:"to,omitempty"`
	LimitType string     `json:"limit_type,omitempty"`
	ResetsAt  *time.Time `json:"resets_at,omitempty"`
	// Reason is "limit" (the harness exited on a plan limit) or
	// "observed" (a fresh plan observation said the model was exhausted).
	Reason  string    `json:"reason"`
	Message string    `json:"message,omitempty"`
	Urgent  bool      `json:"urgent,omitempty"`
	Time    time.Time `json:"time"`
	Reply   string    `json:"reply,omitempty"`
}

// exhaustedReason is the blocked reason when the chain runs out.
func exhaustedReason(resets time.Time) string {
	if resets.IsZero() {
		return "all models exhausted"
	}
	return "all models exhausted (resets " + resets.UTC().Format(time.RFC3339) + ")"
}

// earliestReset is the soonest known reset across limit hits, ignoring a
// reset that was already past when its limit was seen.
func earliestReset(runs []ModelRun) time.Time {
	var t time.Time
	for _, r := range runs {
		if r.Limit == nil || r.Limit.ResetsAt.IsZero() || (!r.End.IsZero() && !r.Limit.ResetsAt.After(r.End)) {
			continue
		}
		if t.IsZero() || r.Limit.ResetsAt.Before(t) {
			t = r.Limit.ResetsAt
		}
	}
	return t
}

// degradeMark is ls's model column: the model running now, with ↓ when the
// step degraded from its order's model.
func degradeMark(order ModelStep, runs []ModelRun) string {
	current := order
	for i := len(runs) - 1; i >= 0; i-- {
		if !runs[i].Skipped {
			current = runs[i].ModelStep
			break
		}
	}
	mark := current.Short()
	if current.Harness != order.Harness || current.Model != order.Model {
		mark += "↓"
	}
	return mark
}
