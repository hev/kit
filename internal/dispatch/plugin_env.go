package dispatch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Plugin-only environment names reach only the plugin exec that declares
// them. Every other child dispatch starts drops them: the round harness, the
// worker supervisor, the worker harness, notification senders and every
// helper command (Git, gh, SSH, tmux). A later exec kind adds its declared
// names to pluginOnlyEnvSources and starts its child with pluginEnvironment.
// The orchestrator supervisor is the one control process that keeps them; it
// moves them out of its own environment with HoldPluginOnlyEnv.
var pluginOnlyEnvSources = []func() []string{sourceEnvNames}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Dispatch and its children cannot run without these, so none can be plugin-only.
var reservedEnvNames = []string{"PATH", "PATHEXT", "HOME", "USERPROFILE", "SYSTEMROOT", "COMSPEC", "TMPDIR", "TEMP", "TMP", "HEV_ORCHESTRATOR", "HEV_DISPATCH_CONFIG"}

// validatePluginEnv checks a declared plugin-only name list.
func validatePluginEnv(field string, names []string) error {
	seen := map[string]bool{}
	for _, n := range names {
		key := strings.ToUpper(n)
		if !envNamePattern.MatchString(n) || seen[key] {
			return fmt.Errorf("%s: invalid or duplicate environment name %q", field, n)
		}
		if slices.Contains(reservedEnvNames, key) {
			return fmt.Errorf("%s: %s cannot be plugin-only", field, n)
		}
		seen[key] = true
	}
	return nil
}

// sourceEnvNames collects source.env from every spec applied on this host,
// including scope-only registrations, so a direct start strips them too.
func sourceEnvNames() []string {
	entries, _ := os.ReadDir(OrchestratorConfigDir())
	var out []string
	for _, f := range entries {
		name, ok := strings.CutSuffix(f.Name(), ".md")
		if !ok {
			continue
		}
		if s, e := LoadOrchestrator(name); e == nil {
			out = append(out, s.Source.Env...)
		}
	}
	return out
}

func pluginOnlyEnvNames() []string {
	var out []string
	for _, names := range pluginOnlyEnvSources {
		for _, n := range names() {
			if envNamePattern.MatchString(n) {
				out = append(out, n)
			}
		}
	}
	return out
}

// withoutPluginOnlyEnv drops plugin-only names, except keep, from env.
// Names compare case-insensitively, as Windows environments do.
func withoutPluginOnlyEnv(env []string, keep ...string) []string {
	var drop []string
	for _, n := range pluginOnlyEnvNames() {
		if !slices.ContainsFunc(keep, func(k string) bool { return strings.EqualFold(k, n) }) {
			drop = append(drop, n)
		}
	}
	if len(drop) == 0 {
		return env
	}
	out := []string{}
	for _, v := range env {
		key, _, _ := strings.Cut(v, "=")
		if !slices.ContainsFunc(drop, func(n string) bool { return strings.EqualFold(key, n) }) {
			out = append(out, v)
		}
	}
	return out
}

var (
	heldMu  sync.Mutex
	heldEnv = map[string]string{}
)

// HoldPluginOnlyEnv moves this process's plugin-only values out of its
// environment, so a child started by any path, filtered or not, cannot
// inherit them. pluginEnvironment still passes a held value to the plugin
// that declares it. The orchestrator supervisor calls this at startup.
func HoldPluginOnlyEnv() {
	heldMu.Lock()
	defer heldMu.Unlock()
	for _, n := range pluginOnlyEnvNames() {
		if v, ok := os.LookupEnv(n); ok {
			heldEnv[strings.ToUpper(n)] = n + "=" + v
			_ = os.Unsetenv(n)
		}
	}
}

// pluginEnvironment is the only child environment carrying plugin-only names,
// and only the names declared for this exec.
func pluginEnvironment(declared []string) []string {
	env := withoutPluginOnlyEnv(orchestratorEnvironment(""), declared...)
	heldMu.Lock()
	defer heldMu.Unlock()
	for _, n := range declared {
		if v, ok := heldEnv[strings.ToUpper(n)]; ok && !hasEnvName(env, n) {
			env = append(env, v)
		}
	}
	return env
}

// controlEnvironment launches an orchestrator supervisor, the one control
// process that keeps plugin-only values, including any this process holds.
func controlEnvironment() []string {
	env := orchestratorEnvironment("")
	heldMu.Lock()
	defer heldMu.Unlock()
	for _, v := range heldEnv {
		if n, _, _ := strings.Cut(v, "="); !hasEnvName(env, n) {
			env = append(env, v)
		}
	}
	return env
}

func hasEnvName(env []string, name string) bool {
	return slices.ContainsFunc(env, func(v string) bool {
		k, _, _ := strings.Cut(v, "=")
		return strings.EqualFold(k, name)
	})
}

// helperCommand starts any non-plugin child (Git, gh, SSH, tmux, hev itself)
// with this process's environment minus plugin-only names.
func helperCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, name, args...)
	c.Env = withoutPluginOnlyEnv(os.Environ())
	return c
}

// helperExec is helperCommand without a context.
func helperExec(name string, args ...string) *exec.Cmd {
	return helperCommand(context.Background(), name, args...)
}

// childEnvironment is orchestratorEnvironment for any non-plugin child.
func childEnvironment(name string) []string {
	return withoutPluginOnlyEnv(orchestratorEnvironment(name))
}
