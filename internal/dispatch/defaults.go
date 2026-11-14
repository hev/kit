package dispatch

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Role string

const (
	RoleWorker       Role = "worker"
	RoleOrchestrator Role = "orchestrator"
	RoleValidation   Role = "validation"
)

// BuiltinHarness and builtinModels are the single source of role defaults.
const BuiltinHarness = "claude"

var builtinModels = map[string]string{"claude": "claude-opus-5-5", "codex": ""}

type HarnessModel struct {
	Harness string `toml:"harness" json:"harness"`
	Model   string `toml:"model" json:"model"`
}
type Resolution struct {
	HarnessModel
	HarnessSource   string `json:"harness_source"`
	ModelSource     string `json:"model_source"`
	HarnessInferred bool   `json:"harness_inferred,omitempty"`
}

func inferredHarness(model string) string {
	if strings.HasPrefix(model, "claude-") {
		return "claude"
	}
	return "codex"
}
func validHarness(h string) bool { _, ok := builtinModels[h]; return h == "" || ok }

// ResolveDefaults pairs harness and model at each layer. A model-only layer
// selects its harness; a harness-only layer never inherits another harness's model.
func ResolveDefaults(c Config, role Role, explicit HarnessModel) (Resolution, error) {
	if role != RoleWorker && role != RoleOrchestrator && role != RoleValidation {
		return Resolution{}, fmt.Errorf("unknown defaults role %q", role)
	}
	r := Resolution{HarnessModel: HarnessModel{BuiltinHarness, builtinModels[BuiltinHarness]}, HarnessSource: "builtin", ModelSource: "builtin"}
	for _, layer := range []struct {
		pair   HarnessModel
		source string
	}{{c.Defaults[role], "node"}, {explicit, "explicit"}} {
		p := layer.pair
		if !validHarness(p.Harness) {
			return r, fmt.Errorf("invalid %s %s harness %q", layer.source, role, p.Harness)
		}
		if p.Harness == "" && p.Model == "" {
			continue
		}
		inferred := p.Harness == ""
		if inferred {
			p.Harness = inferredHarness(p.Model)
		}
		if p.Model != "" {
			r.Model, r.ModelSource = p.Model, layer.source
		} else if p.Harness != r.Harness {
			r.Model, r.ModelSource = builtinModels[p.Harness], "builtin"
		}
		r.Harness, r.HarnessSource, r.HarnessInferred = p.Harness, layer.source, inferred
	}
	return r, nil
}
func ResolvedDefaults(c Config) (map[Role]Resolution, error) {
	out := map[Role]Resolution{}
	for _, role := range []Role{RoleOrchestrator, RoleWorker, RoleValidation} {
		r, err := ResolveDefaults(c, role, HarnessModel{})
		if err != nil {
			return nil, err
		}
		out[role] = r
	}
	return out, nil
}

var validationAliasWarning sync.Once

func normalizeDefaults(c *Config) error {
	if c.Defaults == nil {
		c.Defaults = map[Role]HarnessModel{}
	}
	if c.Validation.Harness != "" || c.Validation.Model != "" {
		validationAliasWarning.Do(func() {
			fmt.Fprintln(os.Stderr, "warning: [validation] harness/model is deprecated; use [defaults.validation]")
		})
		if _, exists := c.Defaults[RoleValidation]; !exists {
			c.Defaults[RoleValidation] = HarnessModel{c.Validation.Harness, c.Validation.Model}
		}
	}
	for role := range c.Defaults {
		if _, err := ResolveDefaults(*c, role, HarnessModel{}); err != nil {
			return err
		}
	}
	return nil
}
func resolveRunDefaults(m *Meta, c Config) error {
	role := RoleWorker
	if m.Parent != "" {
		role = RoleValidation
	} else if m.Until == "all followed runs merged or abandoned" {
		role = RoleOrchestrator
	}
	r, err := ResolveDefaults(c, role, HarnessModel{m.Harness, m.Model})
	if err != nil {
		return err
	}
	m.Harness, m.Model = r.Harness, r.Model
	m.HarnessSource, m.ModelSource, m.HarnessInferred = r.HarnessSource, r.ModelSource, r.HarnessInferred
	return nil
}
func preflightHarness(harness, source string, paths ...string) error {
	path := ""
	if len(paths) > 0 {
		path = paths[0]
	}
	if _, err := exec.LookPath(harness); err != nil {
		return fmt.Errorf("harness preflight: binary %q unavailable on launch PATH=%q (source=%s; config=%s): %w", harness, os.Getenv("PATH"), source, ConfigPath(path), err)
	}
	return nil
}
