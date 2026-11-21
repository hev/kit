package dispatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/harness"
)

// Worker isolation for climb units (RFC 0023, control requirement 1).
//
// An order with env: minimal, or class: climb, which implies it, launches its
// worker harness with a minimal environment instead of the host's launch
// allowlist: the kept names below, the target's [climb] worker_env and a
// scratch HOME in the run directory that holds only the harness's own login.
// The supervisor keeps the ordinary launch environment; only the worker
// harness (and everything it starts) is isolated.
//
// A climb unit's worker carries HEV_RUN_CLASS=climb. Admission refuses
// start, say, rework, stop and the other run-control actions whose caller is a
// climb unit, found by that marker or, on Linux, by the caller's process
// lineage reaching a live climb unit's harness.

const (
	EnvStandard = "standard"
	EnvMinimal  = "minimal"
	ClimbClass  = "climb"

	// RunClassEnv marks a climb unit's worker environment.
	RunClassEnv = "HEV_RUN_CLASS"
	// RunEnvEnv records env: minimal in the worker environment.
	RunEnvEnv = "HEV_RUN_ENV"

	scratchHomeDir = "scratch-home"
)

// MinimalEnv reports whether the order's worker runs with env: minimal.
func (o Order) MinimalEnv() bool { return o.Env == EnvMinimal || o.Class == ClimbClass }

// minimalKeptEnv is what a minimal worker keeps from the host, when set:
// the basics a shell needs and the harness's own login location.
var minimalKeptEnv = []string{"PATH", "LANG", "LC_ALL", "TMPDIR", "SHELL", "CLAUDE_CONFIG_DIR", "CODEX_HOME"}

// minimalWindowsEnv are names Windows processes need to start at all.
var minimalWindowsEnv = []string{"SYSTEMROOT", "COMSPEC", "PATHEXT", "TEMP", "TMP"}

// climbRefusedEnv are names that let a worker start, steer or stop work, push
// to a remote or create paid hosts. A minimal worker never gets them, and
// [climb] worker_env cannot name them.
var climbRefusedEnv = []string{
	"HEV_DISPATCH_CONFIG", "HEV_CONFIG", "SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN",
	"GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GIT_ASKPASS", "SSH_ASKPASS",
	"XDG_CONFIG_HOME", "BOAT_API_KEY",
}

// climbReservedEnv are set by dispatch for a minimal worker and cannot come
// from [climb] worker_env.
var climbReservedEnv = []string{"HOME", "USERPROFILE", RunClassEnv, RunEnvEnv}

func layerEnvName(name string) bool {
	n := strings.ToUpper(name)
	return strings.HasPrefix(n, "HEVLAYER_") || strings.HasPrefix(n, "LAYER_")
}

// layerWriteKey reports a Layer name that carries a credential (a key,
// token, secret or reference to one) other than a read-only one, which says
// READ in its name (for example HEVLAYER_READ_API_KEY).
func layerWriteKey(name string) bool {
	n := strings.ToUpper(name)
	if !layerEnvName(n) {
		return false
	}
	credential := strings.Contains(n, "KEY") || strings.Contains(n, "TOKEN") || strings.Contains(n, "SECRET") || strings.HasSuffix(n, "_REF")
	return credential && !strings.Contains(n, "_READ_") && !strings.HasSuffix(n, "_READ")
}

func containsFold(list []string, name string) bool {
	return slices.ContainsFunc(list, func(s string) bool { return strings.EqualFold(s, name) })
}

// refusedWorkerName reports a name a minimal worker never gets from the host
// unless [climb] worker_env allows it: the refused names and every Layer name.
func refusedWorkerName(name string) bool {
	return containsFold(climbRefusedEnv, name) || layerEnvName(name)
}

// checkClimbWorkerEnv refuses a [climb] worker_env that names a refused,
// reserved or write-key variable. Errors name the variable, never a value.
func checkClimbWorkerEnv(names []string) error {
	for _, n := range names {
		switch {
		case !envNamePattern.MatchString(n):
			return fmt.Errorf("climb.worker_env: invalid environment name %q", n)
		case containsFold(climbRefusedEnv, n):
			return fmt.Errorf("climb.worker_env: %s is refused for climb workers (it can dispatch, push or create hosts)", n)
		case containsFold(climbReservedEnv, n) || strings.HasPrefix(strings.ToUpper(n), "HEV_"):
			return fmt.Errorf("climb.worker_env: %s is reserved for dispatch", n)
		case layerWriteKey(n):
			return fmt.Errorf("climb.worker_env: %s is a Layer write key; allow only a read-only key (named with READ)", n)
		}
	}
	return nil
}

// resolveMinimalLaunch records the names a minimal worker gets. Kept names
// are recorded when set in the admitting process; worker_env names always,
// like host env. Host env is not forwarded to a minimal worker.
func resolveMinimalLaunch(m *Meta, cfg Config) error {
	if err := checkClimbWorkerEnv(cfg.Climb.WorkerEnv); err != nil {
		return err
	}
	kept := slices.Clone(minimalKeptEnv)
	if runtime.GOOS == "windows" {
		kept = append(kept, minimalWindowsEnv...)
	}
	names := map[string]bool{}
	for _, key := range kept {
		if _, ok := os.LookupEnv(key); ok {
			names[key] = true
		}
	}
	for _, key := range cfg.Climb.WorkerEnv {
		names[key] = true
	}
	out := make([]string, 0, len(names))
	for key := range names {
		out = append(out, key)
	}
	sort.Strings(out)
	m.WorkerEnv = out
	return nil
}

// harnessLoginPaths are the paths under the real HOME that hold a harness's
// own login, linked into the scratch HOME. A harness whose login directory
// is set by environment (CLAUDE_CONFIG_DIR, CODEX_HOME) needs no link.
func harnessLoginPaths(name string, env []string) []string {
	switch name {
	case harness.Claude:
		if !hasEnvName(env, "CLAUDE_CONFIG_DIR") {
			return []string{".claude", ".claude.json"}
		}
	case harness.Codex:
		if !hasEnvName(env, "CODEX_HOME") {
			return []string{".codex"}
		}
	case harness.Agy:
		return []string{".gemini"}
	}
	return nil
}

// gitIdentity reads the operator's commit identity, so a worker can commit
// in its worktree without the real global git config.
var gitIdentity = func() (name, email string) {
	read := func(key string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := helperCommand(ctx, "git", "config", "--global", "--get", key).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return read("user.name"), read("user.email")
}

// gitConfigValue quotes a value for a git config file.
func gitConfigValue(v string) string {
	v = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", " ", "\r", " ", "\x00", "").Replace(v)
	return "\"" + v + "\""
}

// prepareScratchHome makes a fresh scratch HOME for one attempt: mode 0700,
// the selected harness's login linked in, a .gitconfig with the commit
// identity and no credential helper, and nothing else: no .config/hev,
// .config/gh, .ssh or git credentials. A previous attempt's scratch HOME is
// removed first; links are removed, never followed.
func prepareScratchHome(dir, harnessName string, env []string) (string, error) {
	home := filepath.Join(dir, scratchHomeDir)
	if err := os.RemoveAll(home); err != nil {
		return "", fmt.Errorf("scratch home: %w", err)
	}
	if err := os.Mkdir(home, 0700); err != nil {
		return "", fmt.Errorf("scratch home: %w", err)
	}
	if real, err := os.UserHomeDir(); err == nil && real != "" {
		for _, p := range harnessLoginPaths(harnessName, env) {
			src := filepath.Join(real, p)
			if _, err := os.Lstat(src); err != nil {
				continue
			}
			if err := os.Symlink(src, filepath.Join(home, p)); err != nil {
				return "", fmt.Errorf("scratch home: link harness login %s: %w", p, err)
			}
		}
	}
	name, email := gitIdentity()
	cfg := "[credential]\n\thelper =\n"
	if name != "" || email != "" {
		cfg = "[user]\n"
		if name != "" {
			cfg += "\tname = " + gitConfigValue(name) + "\n"
		}
		if email != "" {
			cfg += "\temail = " + gitConfigValue(email) + "\n"
		}
		cfg += "[credential]\n\thelper =\n"
	}
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(cfg), 0600); err != nil {
		return "", fmt.Errorf("scratch home: %w", err)
	}
	return home, nil
}

// minimalWorkerEnvironment is a minimal worker's environment, built from
// nothing rather than filtered from the supervisor's: the recorded worker
// names, a scratch HOME, the run markers and trace context.
func minimalWorkerEnvironment(dir string, m Meta) ([]string, error) {
	env := []string{}
	for _, key := range m.WorkerEnv {
		if strings.ContainsAny(key, "=\x00") {
			return nil, fmt.Errorf("invalid worker environment name")
		}
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	home, err := prepareScratchHome(dir, m.Harness, env)
	if err != nil {
		return nil, err
	}
	env = append(env, "HOME="+home)
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+home)
	}
	env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", RunEnvEnv+"="+EnvMinimal)
	if m.Class == ClimbClass {
		env = append(env, RunClassEnv+"="+ClimbClass)
	}
	env = withoutPluginOnlyEnv(env)
	env = append(env, "HEV_REQUESTED_BY="+requester(m))
	return append(env, traceEnvironment(m)...), nil
}

// withoutRefusedWorkerEnv is the last pass over a minimal worker's
// environment, after harness settings are added: it drops every refused name
// that [climb] worker_env did not allow.
func withoutRefusedWorkerEnv(env, allowed []string) []string {
	out := env[:0:0]
	for _, v := range env {
		key, _, _ := strings.Cut(v, "=")
		if refusedWorkerName(key) && !containsFold(allowed, key) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// climbControlActions are the requests a climb unit may not make: anything
// that starts, steers or stops work. Reads (list, show, logs) stay open.
var climbControlActions = []string{
	"start", "rework", "retry", "amend", "say", "peer-send", "steer", "stop",
	"pause", "resume", "wake", "retire", "close", "clean", "adopt", "orchestrator-apply",
}

// climbCallerUnit names the climb unit this process runs inside, or
// "climb" when only the environment marker says so; "" outside one.
func climbCallerUnit() string {
	if unit := climbUnitAncestor(); unit != "" {
		return unit
	}
	if strings.EqualFold(os.Getenv(RunClassEnv), ClimbClass) {
		return ClimbClass
	}
	return ""
}

// climbUnitAncestor finds a live climb unit whose recorded harness process is
// an ancestor of this one, so a worker that clears its environment is still
// recognized. Linux only; elsewhere processAncestors is empty.
func climbUnitAncestor() string {
	ancestors := processAncestors()
	if len(ancestors) == 0 {
		return ""
	}
	entries, err := os.ReadDir(Root())
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(Root(), e.Name())
		path := filepath.Join(dir, "harness.json")
		var p pidRecord
		if readJSON(path, &p) != nil || !ancestors[p.PID] {
			continue
		}
		var m Meta
		if readJSON(filepath.Join(dir, "meta.json"), &m) != nil || m.Class != ClimbClass {
			continue
		}
		if processRecordLive(path) {
			return e.Name()
		}
	}
	return ""
}

// markCallerClass records on a request that its caller is a climb unit. It
// never clears the mark, so a forwarded request keeps it.
func markCallerClass(q *Request) {
	if q.CallerClass == "" && slices.Contains(climbControlActions, q.Action) && climbCallerUnit() != "" {
		q.CallerClass = ClimbClass
	}
}

// admitCaller refuses run control from inside a climb unit.
func admitCaller(q Request) error {
	if !slices.Contains(climbControlActions, q.Action) {
		return nil
	}
	unit := climbCallerUnit()
	if unit == "" && q.CallerClass == ClimbClass {
		unit = ClimbClass
	}
	if unit == "" {
		return nil
	}
	who := "a climb unit"
	if unit != ClimbClass {
		who = "climb unit " + unit
	}
	return fmt.Errorf("refused: %s from inside %s; climb workers cannot start, steer or stop dispatch work (RFC 0023, control requirement 1)", q.Action, who)
}
