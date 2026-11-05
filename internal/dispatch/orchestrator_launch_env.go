package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// A scheduled task starts with the user's profile environment, not the
// caller's. These non-secret variables choose the dispatch root, configuration
// and harness homes, so a scheduled launch carries them as --launch-env
// arguments. Secrets are never put on a command line.
var launchCarried = []string{"HOME", "USERPROFILE", "PATH", "HEV_DISPATCH_CONFIG", "HEV_CONFIG", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "HEV_PEERS_BACKEND", "HEV_PEERS_STATE", "PEERS_SELF", "LANG", "LC_ALL", "TMPDIR", "SHELL", "LAYER_NAMESPACE"}

// launchSessionOnly describes the caller's session or shell, not what the
// supervisor runs with. The launcher sets HEV_ORCHESTRATOR itself.
var launchSessionOnly = []string{"HEV_ORCHESTRATOR", "SSH_CLIENT", "SSH_CONNECTION", "SSH_TTY", "SSH_AUTH_SOCK", "TERM", "PROMPT", "PSModulePath", "PWD", "OLDPWD", "SHLVL", "_"}

// launchPaths name filesystem locations; PATH is a list of them. A task starts
// in the orchestrator directory, not the caller's, so a relative value would
// name a different file there. The launcher refuses relative values rather
// than rewriting them: it cannot know every reader's base directory or whether
// it expands "~", and an error naming the variable is easy to act on.
var launchPaths = []string{"HOME", "USERPROFILE", "HEV_DISPATCH_CONFIG", "HEV_CONFIG", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "HEV_PEERS_STATE", "TMPDIR"}

// scheduledLaunchEnvironment compares the caller's environment with the
// profile environment a scheduled task would start with. carried holds the
// --launch-env values that reproduce the caller's context; a name without "="
// unsets it. refused names every other difference, which a task cannot
// reproduce. Names compare case-insensitively, as on Windows.
func scheduledLaunchEnvironment(caller, profile []string) (carried, refused []string) {
	have, want := envIndex(profile), envIndex(caller)
	keys := map[string]bool{}
	for k := range have {
		keys[k] = true
	}
	for k := range want {
		keys[k] = true
	}
	for k := range keys {
		w, h := want[k], have[k]
		if w.set && launchRelative(k, w.value) {
			// Even a value the profile shares changes meaning with the directory.
			refused = append(refused, w.name+" (relative path)")
			continue
		}
		if w == h || strings.HasPrefix(k, "=") || launchListed(launchSessionOnly, k) {
			continue
		}
		name := w.name
		if name == "" {
			name = h.name
		}
		switch {
		case !launchListed(launchCarried, k):
			refused = append(refused, name)
		case w.set:
			carried = append(carried, name+"="+w.value)
		default:
			carried = append(carried, name)
		}
	}
	sort.Strings(carried)
	sort.Strings(refused)
	return carried, refused
}

type envEntry struct {
	name, value string
	set         bool
}

func envIndex(env []string) map[string]envEntry {
	m := map[string]envEntry{}
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		if name == "" {
			// Windows per-drive directories look like "=C:=C:\dir".
			name, value, _ = strings.Cut(kv[1:], "=")
			name = "=" + name
		}
		m[strings.ToUpper(name)] = envEntry{name, value, true}
	}
	return m
}

func launchListed(list []string, upper string) bool {
	for _, k := range list {
		if strings.ToUpper(k) == upper {
			return true
		}
	}
	return false
}

// launchRelative reports whether a path-valued variable depends on the working
// directory. PATH splits on ";" because only Windows launches scheduled tasks;
// Windows skips empty PATH entries.
func launchRelative(upper, value string) bool {
	var paths []string
	switch {
	case upper == "PATH":
		paths = strings.Split(value, ";")
	case launchListed(launchPaths, upper):
		paths = []string{value}
	}
	for _, p := range paths {
		if p != "" && !filepath.IsAbs(p) && !trace.WindowsPath(p) {
			return true
		}
	}
	return false
}

// ApplyLaunchEnvironment restores the context a scheduled launch carried.
func ApplyLaunchEnvironment(carried []string) error {
	for _, kv := range carried {
		name, value, set := strings.Cut(kv, "=")
		if !launchListed(launchCarried, strings.ToUpper(name)) {
			return fmt.Errorf("launch environment cannot carry %q", name)
		}
		var e error
		if set {
			e = os.Setenv(name, value)
		} else {
			e = os.Unsetenv(name)
		}
		if e != nil {
			return e
		}
	}
	return nil
}
