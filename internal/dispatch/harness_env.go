package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/hevpaths"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/workspace"
)

// HarnessConfig is one optional [harnesses.NAME] table in dispatch.toml.
//
// EnvRefs maps environment names to secret references (op://, azkv:// or
// secret://). They are resolved through kit's secrets client when that
// harness launches and reach only the harness's environment: never meta.json,
// the order, the log or an error message.
//
// ModelProvider (agy only) names the provider agy is told to use with
// --settings '{"modelProvider":"NAME"}'. Without it, agy still gets
// "gemini" whenever GEMINI_API_KEY is set in its environment.
type HarnessConfig struct {
	EnvRefs       map[string]string `toml:"env_refs"`
	ModelProvider string            `toml:"model_provider"`
}

// harnessSecretTTL is how long the secrets cache may keep an env_refs value.
const harnessSecretTTL = time.Hour

var providerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// checkHarnesses validates [harnesses.*]. Messages name the key, never a
// reference's value.
func checkHarnesses(hs map[string]HarnessConfig) error {
	names := make([]string, 0, len(hs))
	for n := range hs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h := hs[n]
		if _, ok := harness.DispatchFor(n); !ok {
			return fmt.Errorf("harnesses.%s: unknown harness; want codex, claude or agy", n)
		}
		for env, ref := range h.EnvRefs {
			if !envNamePattern.MatchString(env) {
				return fmt.Errorf("harnesses.%s.env_refs: invalid environment name %q", n, env)
			}
			if !secrets.ValidRef(ref) && !secrets.ValidNeutral(ref) {
				return fmt.Errorf("harnesses.%s.env_refs.%s: not a secret reference (op://, azkv:// or secret://)", n, env)
			}
		}
		if h.ModelProvider != "" {
			if n != "agy" {
				return fmt.Errorf("harnesses.%s.model_provider: only agy takes a model provider", n)
			}
			if !providerPattern.MatchString(h.ModelProvider) {
				return fmt.Errorf("harnesses.%s.model_provider: invalid provider %q", n, h.ModelProvider)
			}
		}
	}
	return nil
}

// harnessSettings reads only [harnesses.HARNESS] from the dispatch config at
// launch, so an unrelated host entry cannot stop a harness from starting. A
// missing config file means no settings.
func harnessSettings(path, harness string) (HarnessConfig, error) {
	var raw struct {
		Harnesses map[string]HarnessConfig `toml:"harnesses"`
	}
	md, err := toml.DecodeFile(ConfigPath(path), &raw)
	if os.IsNotExist(err) {
		return HarnessConfig{}, nil
	}
	if err != nil {
		return HarnessConfig{}, err
	}
	for _, key := range md.Undecoded() {
		if len(key) > 0 && key[0] == "harnesses" {
			return HarnessConfig{}, fmt.Errorf("unknown config key: %s", key)
		}
	}
	if err := checkHarnesses(raw.Harnesses); err != nil {
		return HarnessConfig{}, err
	}
	return raw.Harnesses[harness], nil
}

// resolveHarnessSecrets is the secrets client call; tests replace it.
var resolveHarnessSecrets = func(ctx context.Context, refs []secrets.Ref, caller string) (map[string]string, error) {
	return secrets.ResolveWith(ctx, refs, secrets.Options{Caller: caller})
}

// harnessSecretEnv resolves h.EnvRefs to NAME=VALUE entries, sorted by name,
// and returns the resolved values for redaction. Errors name the harness and
// the environment names only, never a reference or a value.
func harnessSecretEnv(ctx context.Context, harness string, h HarnessConfig, caller string) ([]string, []string, error) {
	if len(h.EnvRefs) == 0 {
		return nil, nil, nil
	}
	names := make([]string, 0, len(h.EnvRefs))
	for n := range h.EnvRefs {
		names = append(names, n)
	}
	sort.Strings(names)
	// Plugin-only names reach only the source plugin that declares them;
	// env_refs cannot put one back into a harness environment.
	plugin := pluginOnlyEnvNames()
	for _, n := range names {
		for _, p := range plugin {
			if strings.EqualFold(n, p) {
				return nil, nil, fmt.Errorf("harnesses.%s.env_refs.%s: reserved for a source plugin (source.env %s)", harness, n, p)
			}
		}
	}
	refs := make([]secrets.Ref, 0, len(names))
	for _, n := range names {
		refs = append(refs, secrets.Ref{Name: n, URI: h.EnvRefs[n], TTL: harnessSecretTTL})
	}
	values, err := resolveHarnessSecrets(ctx, refs, caller)
	if err != nil {
		return nil, nil, fmt.Errorf("harnesses.%s.env_refs (%s): %w", harness, strings.Join(names, ", "), err)
	}
	out := make([]string, 0, len(names))
	resolved := make([]string, 0, len(names))
	for _, n := range names {
		v, ok := values[n]
		if !ok {
			return nil, nil, fmt.Errorf("harnesses.%s.env_refs: %s did not resolve", harness, n)
		}
		out = append(out, n+"="+v)
		resolved = append(resolved, v)
	}
	return out, resolved, nil
}

// envFoldNames is true where environment names match case-insensitively, as
// os/exec deduplicates them on Windows.
var envFoldNames = runtime.GOOS == "windows"

// envValue is the effective value of name in env: the last matching entry
// wins, as it does for exec.Cmd.Env, and names match the way os/exec matches
// them on this platform.
func envValue(env []string, name string) string {
	v := ""
	for _, kv := range env {
		k, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if k == name || (envFoldNames && strings.EqualFold(k, name)) {
			v = val
		}
	}
	return v
}

// harnessModelProvider is the provider agy is launched with: the configured
// one, else gemini when GEMINI_API_KEY is set, else none. Other harnesses
// have none.
func harnessModelProvider(harness string, h HarnessConfig, env []string) string {
	if harness != "agy" {
		return ""
	}
	if h.ModelProvider != "" {
		return h.ModelProvider
	}
	if envValue(env, "GEMINI_API_KEY") != "" {
		return "gemini"
	}
	return ""
}

// withModelProvider adds --settings {"modelProvider":PROVIDER} just before
// the prompt (agy's --print=PROMPT), which stays the last element.
func withModelProvider(args []string, provider string) []string {
	if provider == "" || len(args) == 0 {
		return args
	}
	b, _ := json.Marshal(map[string]string{"modelProvider": provider})
	out := append([]string{}, args[:len(args)-1]...)
	out = append(out, "--settings", string(b))
	return append(out, args[len(args)-1])
}

// harnessLaunch applies [harnesses.HARNESS] to one launch: resolved env_refs
// are appended to env (overriding inherited names) and agy's model provider
// is added to args. It also returns the resolved values, which the caller
// redacts from the harness's output. With no table, env and args come back
// unchanged.
func harnessLaunch(ctx context.Context, configPath, harness, caller string, env, args []string) ([]string, []string, []string, error) {
	return workspaceHarnessLaunch("", ctx, configPath, harness, caller, env, args)
}

func workspaceHarnessLaunch(name string, ctx context.Context, configPath, harness, caller string, env, args []string) ([]string, []string, []string, error) {
	h, err := harnessSettings(configPath, harness)
	if err != nil {
		return env, args, nil, err
	}
	if name != "" {
		w, err := workspace.Named(name)
		if err != nil {
			return env, args, nil, err
		}
		if name != workspace.Default || w.Login == "own" || w.Subscriptions.EnvRefs != nil {
			h.EnvRefs = w.Subscriptions.EnvRefs
			for key := range h.EnvRefs {
				for _, reserved := range []string{
					"HEV_WORKSPACE", workspace.HostHomeEnv, "HEV_CONFIG", "HEV_DISPATCH_CONFIG",
					"HEV_ORDERS_DIR", "HEV_PEERS_STATE", secrets.EndpointEnv,
					"CLAUDE_CONFIG_DIR", "CODEX_HOME", "HEV_AGY_DATA_DIR", "GEMINI_CLI_HOME",
					"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA",
					"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
				} {
					if strings.EqualFold(key, reserved) {
						return env, args, nil, fmt.Errorf("workspace subscription environment name %q is reserved", key)
					}
				}
			}
			if err := checkHarnesses(map[string]HarnessConfig{harness: h}); err != nil {
				return env, args, nil, err
			}
		}
		if w.IsolatedStorage() {
			blocked := append(layerenv.Names(), "FACTORY_BOARD_URL", "FACTORY_BOARD_LAYER_API_KEY", "FACTORY_BOARD_NAMESPACE")
			env = workspaceEnvWithout(env, blocked)
		}
		profile := w.Profile(harness)
		if profile != "" {
			var blocked []string
			switch harness {
			case "claude":
				blocked = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}
			case "codex":
				blocked = []string{"OPENAI_API_KEY", "CODEX_API_KEY"}
			case "agy":
				blocked = []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"}
			}
			env = workspaceEnvWithout(env, blocked)
			if err := os.MkdirAll(profile, 0700); err != nil {
				return env, args, nil, fmt.Errorf("could not create workspace %q harness profile", name)
			}
			switch harness {
			case "claude":
				env = append(env, "CLAUDE_CONFIG_DIR="+profile)
			case "codex":
				env = append(env, "CODEX_HOME="+profile)
			case "agy":
				// Antigravity reads OAuth, settings and sessions under its home.
				// Keep kit's control files reachable with explicit host paths.
				env = append(env,
					"HOME="+profile, "USERPROFILE="+profile,
					"APPDATA="+filepath.Join(profile, "AppData", "Roaming"),
					"LOCALAPPDATA="+filepath.Join(profile, "AppData", "Local"),
					"XDG_CONFIG_HOME="+filepath.Join(profile, ".config"),
					"XDG_DATA_HOME="+filepath.Join(profile, ".local", "share"),
					"XDG_CACHE_HOME="+filepath.Join(profile, ".cache"),
					"HEV_AGY_DATA_DIR="+w.AgyTranscriptDir(),
					workspace.HostHomeEnv+"="+workspace.HostHome(),
					"HEV_CONFIG="+workspace.ConfigPath(),
					"HEV_DISPATCH_CONFIG="+ConfigPath(configPath),
					"HEV_ORDERS_DIR="+Root(),
					"HEV_PEERS_STATE="+hevpaths.Dir(hevpaths.Peers),
					secrets.EndpointEnv+"="+secrets.Endpoint(),
				)
			}
		}
		env = append(env, "HEV_WORKSPACE="+name)
	}
	extra, values, err := harnessSecretEnv(ctx, harness, h, caller)
	if err != nil {
		if name != "" && name != workspace.Default {
			return env, args, nil, fmt.Errorf("could not resolve workspace %q harness subscriptions", name)
		}
		return env, args, nil, err
	}
	env = append(env, extra...)
	return env, withModelProvider(args, harnessModelProvider(harness, h, env)), values, nil
}

// redactedMarker replaces a resolved secret value in harness output.
const redactedMarker = "[redacted]"

// harnessOutputWaitDelay bounds how long a redacted harness's descendants may
// hold its output pipe open after the harness exits.
const harnessOutputWaitDelay = 5 * time.Second

// redactingWriter copies harness output to w with every secret value
// replaced. It buffers raw input and decides each byte only once no value,
// complete or still arriving, can cover it: occurrences of all values
// (overlapping ones included) are merged into spans, each span is written
// as one marker, and a tail that may begin a longer value, or a span that
// such a tail may extend, is held for the next write. Close writes what is
// held.
type redactingWriter struct {
	mu     sync.Mutex
	w      io.Writer
	values [][]byte
	// pending is raw, undecided output. Its first cont bytes continue a
	// span whose marker was already written when the buffer hit max.
	pending []byte
	cont    int
	longest int
	// max bounds pending: past it, everything before the earliest
	// possible start of an incomplete value is written.
	max int
}

// redactMaxPending caps the raw output a redactingWriter holds back while
// overlapping values keep a span open.
const redactMaxPending = 64 << 10

// newRedactingWriter returns nil when there is nothing to redact.
func newRedactingWriter(w io.Writer, values []string) *redactingWriter {
	seen := map[string]bool{}
	r := &redactingWriter{w: w, max: redactMaxPending}
	for _, v := range values {
		if v != "" && !seen[v] {
			seen[v] = true
			r.values = append(r.values, []byte(v))
			r.longest = max(r.longest, len(v))
		}
	}
	if len(r.values) == 0 {
		return nil
	}
	return r
}

// spans returns the merged, sorted byte ranges of pending covered by a
// complete value or by the carried continuation.
func (r *redactingWriter) spans() [][2]int {
	var s [][2]int
	if r.cont > 0 {
		s = append(s, [2]int{0, r.cont})
	}
	for _, v := range r.values {
		for off := 0; ; {
			i := bytes.Index(r.pending[off:], v)
			if i < 0 {
				break
			}
			s = append(s, [2]int{off + i, off + i + len(v)})
			off += i + 1
		}
	}
	sort.Slice(s, func(i, j int) bool { return s[i][0] < s[j][0] })
	var merged [][2]int
	for _, x := range s {
		if n := len(merged); n > 0 && x[0] < merged[n-1][1] {
			merged[n-1][1] = max(merged[n-1][1], x[1])
			continue
		}
		merged = append(merged, x)
	}
	return merged
}

// partial returns the earliest offset at which the rest of pending is a
// proper prefix of some value, or len(pending).
func (r *redactingWriter) partial() int {
	for j := max(0, len(r.pending)-r.longest+1); j < len(r.pending); j++ {
		rest := r.pending[j:]
		for _, v := range r.values {
			if len(rest) < len(v) && bytes.HasPrefix(v, rest) {
				return j
			}
		}
	}
	return len(r.pending)
}

// flush writes pending[:cut], each span in it as one marker, and keeps
// the rest. A span cut short carries over as a continuation.
func (r *redactingWriter) flush(cut int, spans [][2]int) error {
	if cut <= 0 {
		return nil
	}
	var out []byte
	at, cont := 0, 0
	for _, s := range spans {
		if s[0] >= cut {
			break
		}
		out = append(out, r.pending[at:s[0]]...)
		if s[0] > 0 || r.cont == 0 {
			out = append(out, redactedMarker...)
		}
		at = min(s[1], cut)
		if s[1] > cut {
			cont = s[1] - cut
		}
	}
	if at < cut {
		out = append(out, r.pending[at:cut]...)
	}
	r.pending = append(r.pending[:0], r.pending[cut:]...)
	r.cont = cont
	_, err := r.w.Write(out)
	return err
}

func (r *redactingWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, p...)
	spans := r.spans()
	hold := r.partial()
	cut := hold
	// A value still arriving at hold may overlap a span that reaches past
	// it, so that span waits too.
	for _, s := range spans {
		if s[0] < cut && cut < s[1] {
			cut = s[0]
		}
	}
	if len(r.pending)-cut > r.max {
		cut = hold
	}
	if err := r.flush(cut, spans); err != nil {
		return len(p), err
	}
	return len(p), nil
}

// Close writes the held tail. It does not close w.
func (r *redactingWriter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.flush(len(r.pending), r.spans())
	r.pending, r.cont = nil, 0
	return err
}

// harnessOutput connects c's stdout and stderr to out, through a redacting
// writer when values is non-empty. The returned wait waits for c and the
// output copy, and flushes the redactor; a pipe held open by a descendant
// past harnessOutputWaitDelay is not an error.
func harnessOutput(c *exec.Cmd, out io.Writer, values []string) func() error {
	r := newRedactingWriter(out, values)
	if r == nil {
		c.Stdout, c.Stderr = out, out
		return c.Wait
	}
	// One writer for both streams: os/exec then copies them through one pipe.
	c.Stdout, c.Stderr = r, r
	c.WaitDelay = harnessOutputWaitDelay
	return func() error {
		err := c.Wait()
		if errors.Is(err, exec.ErrWaitDelay) {
			err = nil
		}
		if cerr := r.Close(); err == nil {
			err = cerr
		}
		return err
	}
}

func workspaceEnvWithout(env, blocked []string) []string {
	out := make([]string, 0, len(env))
entries:
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		for _, key := range blocked {
			if strings.EqualFold(name, key) {
				continue entries
			}
		}
		out = append(out, entry)
	}
	return out
}
