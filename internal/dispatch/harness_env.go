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
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/secrets"
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
		if !ValidHarness(n) {
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
	h, err := harnessSettings(configPath, harness)
	if err != nil {
		return env, args, nil, err
	}
	extra, values, err := harnessSecretEnv(ctx, harness, h, caller)
	if err != nil {
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
// replaced. It holds back the last len(longest)-1 bytes so a value split
// across writes is still caught; Close writes what is held.
type redactingWriter struct {
	mu      sync.Mutex
	w       io.Writer
	values  [][]byte
	keep    int
	pending []byte
}

// newRedactingWriter returns nil when there is nothing to redact.
func newRedactingWriter(w io.Writer, values []string) *redactingWriter {
	seen := map[string]bool{}
	var vs []string
	for _, v := range values {
		if v != "" && !seen[v] {
			seen[v] = true
			vs = append(vs, v)
		}
	}
	if len(vs) == 0 {
		return nil
	}
	// Longest first, so a value containing another is replaced whole.
	sort.Slice(vs, func(i, j int) bool { return len(vs[i]) > len(vs[j]) })
	r := &redactingWriter{w: w, keep: len(vs[0]) - 1}
	for _, v := range vs {
		r.values = append(r.values, []byte(v))
	}
	return r
}

func (r *redactingWriter) redact() {
	for _, v := range r.values {
		if bytes.Contains(r.pending, v) {
			r.pending = bytes.ReplaceAll(r.pending, v, []byte(redactedMarker))
		}
	}
}

func (r *redactingWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, p...)
	r.redact()
	// Any value not yet replaced must start in the last keep bytes.
	if n := len(r.pending) - r.keep; n > 0 {
		_, err := r.w.Write(r.pending[:n])
		r.pending = append(r.pending[:0], r.pending[n:]...)
		if err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// Close writes the held tail. It does not close w.
func (r *redactingWriter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.redact()
	if len(r.pending) == 0 {
		return nil
	}
	_, err := r.w.Write(r.pending)
	r.pending = nil
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
