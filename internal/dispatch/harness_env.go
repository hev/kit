package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/secrets"
)

// HarnessConfig is one optional [harnesses.NAME] table in dispatch.toml.
//
// EnvRefs maps environment names to secret references (op://, azkv:// or
// secret://). They are resolved through kit's secrets client when that
// harness launches and reach only the harness's environment: never meta.json,
// the order, the log or an error message.
//
// Copilot can use COPILOT_GITHUB_TOKEN, GH_TOKEN or GITHUB_TOKEN via EnvRefs.
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
			return fmt.Errorf("harnesses.%s: unsupported dispatch harness", n)
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

// harnessSecretEnv resolves h.EnvRefs to NAME=VALUE entries, sorted by name.
// An error names the harness and the missing name only.
func harnessSecretEnv(ctx context.Context, harness string, h HarnessConfig, caller string) ([]string, error) {
	if len(h.EnvRefs) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(h.EnvRefs))
	for n := range h.EnvRefs {
		names = append(names, n)
	}
	sort.Strings(names)
	refs := make([]secrets.Ref, 0, len(names))
	for _, n := range names {
		refs = append(refs, secrets.Ref{Name: n, URI: h.EnvRefs[n], TTL: harnessSecretTTL})
	}
	values, err := resolveHarnessSecrets(ctx, refs, caller)
	if err != nil {
		return nil, fmt.Errorf("harnesses.%s.env_refs: %w", harness, err)
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		v, ok := values[n]
		if !ok {
			return nil, fmt.Errorf("harnesses.%s.env_refs: %s did not resolve", harness, n)
		}
		out = append(out, n+"="+v)
	}
	return out, nil
}

// envValue is the effective value of name in env: the last entry wins, as it
// does for exec.Cmd.Env.
func envValue(env []string, name string) string {
	v := ""
	for _, kv := range env {
		if k, val, ok := strings.Cut(kv, "="); ok && k == name {
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
// is added to args. With no table, env and args come back unchanged.
func harnessLaunch(ctx context.Context, configPath, harness, caller string, env, args []string) ([]string, []string, error) {
	h, err := harnessSettings(configPath, harness)
	if err != nil {
		return env, args, err
	}
	extra, err := harnessSecretEnv(ctx, harness, h, caller)
	if err != nil {
		return env, args, err
	}
	env = append(env, extra...)
	return env, withModelProvider(args, harnessModelProvider(harness, h, env)), nil
}
