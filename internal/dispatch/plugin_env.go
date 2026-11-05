package dispatch

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

// Plugin-only environment names reach only the plugin exec that declares
// them. Every other child dispatch starts drops them: the round harness, the
// worker supervisor, the worker harness and notification senders. A later
// exec kind adds its declared names to pluginOnlyEnvSources and starts its
// child with pluginEnvironment.
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

// pluginEnvironment is the only child environment carrying plugin-only names,
// and only the names declared for this exec.
func pluginEnvironment(declared []string) []string {
	return withoutPluginOnlyEnv(orchestratorEnvironment(""), declared...)
}

// childEnvironment is orchestratorEnvironment for any non-plugin child.
func childEnvironment(name string) []string {
	return withoutPluginOnlyEnv(orchestratorEnvironment(name))
}
