package foh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/secrets"
)

// DefaultBaseURL is OpenRouter's OpenAI-compatible API root.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

// DefaultKitServiceURL is the default Club Kit service API root.
const DefaultKitServiceURL = "https://kit.hevmind.com"

// InferenceMode identifies which inference backend a resolved key targets.
type InferenceMode string

const (
	InferenceModeOpenRouter InferenceMode = "openrouter"
	InferenceModeClubKit    InferenceMode = "club-kit"
)

// InferenceEndpoint is the resolved credential, base URL, and routing mode
// for OpenAI-compatible inference requests.
type InferenceEndpoint struct {
	Mode    InferenceMode
	BaseURL string
	APIKey  string
}

// KitConfig is the optional [kit] sign-in table in ~/.hev/config.toml.
type KitConfig struct {
	Service string `toml:"service"`
	API     string `toml:"api"`
	Key     string `toml:"key"`
}

// DefaultModel is used when no flag, environment, config or favorite names one.
const DefaultModel = "anthropic/claude-sonnet-5.5"

// DefaultDocsURL is where the harness fetches its own documentation.
// Overridable at build time via -ldflags or at runtime via HEV_DOCS_URL / [foh].docs_url.
var DefaultDocsURL = "https://hevkit.com/docs"

// Config is the optional [foh] table in ~/.hev/config.toml.
type Config struct {
	Model        string     `toml:"model"`
	BaseURL      string     `toml:"base_url"`
	APIKeyRef    string     `toml:"api_key_ref"`
	SystemPrompt string     `toml:"system_prompt"`
	DocsURL      string     `toml:"docs_url"`
	SummaryModel string     `toml:"summary_model"`
	KeyScopes    []KeyScope `toml:"key_scopes"`
	// Codex is the optional [foh.codex] table for the codex-sub provider.
	Codex CodexConfig `toml:"codex"`
	// Kit is the optional top-level [kit] sign-in table loaded alongside [foh].
	Kit KitConfig `toml:"-"`
}

// CodexConfig is [foh.codex]: the OpenAI/Codex ChatGPT subscription provider.
type CodexConfig struct {
	// Enabled forces the provider on (true) or off (false). Unset means on
	// when a ChatGPT login is found in the Codex auth file.
	Enabled *bool `toml:"enabled"`
	// Models lists upstream Codex model names; the picker shows each as
	// codex/NAME. Empty uses DefaultCodexModels.
	Models []string `toml:"models"`
	// BaseURL overrides the ChatGPT Codex backend (HEV_FOH_CODEX_BASE_URL wins).
	BaseURL string `toml:"base_url"`
	// Home overrides the Codex home that holds auth.json (CODEX_HOME is next).
	Home string `toml:"home"`
}

// ConfigPath is HEV_CONFIG or ~/.hev/config.toml.
func ConfigPath() string {
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(homeDir(), ".hev", "config.toml")
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// LoadConfig reads [foh] and optional [kit] from path (ConfigPath when empty).
// A missing file or table is an empty Config. Errors never carry the path or any value.
func LoadConfig(path string) (Config, error) {
	if path == "" {
		path = ConfigPath()
	}
	var file struct {
		FOH Config    `toml:"foh"`
		Kit KitConfig `toml:"kit"`
	}
	md, err := toml.DecodeFile(path, &file)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		var p toml.ParseError
		if errors.As(err, &p) {
			return Config{}, fmt.Errorf("invalid foh configuration at line %d", p.Position.Line)
		}
		return Config{}, errors.New("could not read foh configuration")
	}
	for _, key := range md.Undecoded() {
		if len(key) > 2 && key[0] == "foh" && key[1] == "key_scopes" {
			return Config{}, errors.New("unknown field in [[foh.key_scopes]]")
		}
		if len(key) > 2 && key[0] == "foh" && key[1] == "codex" {
			return Config{}, fmt.Errorf("unknown field in [foh.codex]: %s", key[2])
		}
		if len(key) > 1 && key[0] == "foh" {
			return Config{}, fmt.Errorf("unknown field in [foh]: %s", key[1])
		}
	}
	cfg := file.FOH
	cfg.Kit = file.Kit
	return cfg, nil
}

// ResolvedBaseURL resolves HEV_FOH_BASE_URL, then [foh].base_url, then OpenRouter.
func (c Config) ResolvedBaseURL() string {
	for _, v := range []string{os.Getenv("HEV_FOH_BASE_URL"), c.BaseURL} {
		if v = strings.TrimSpace(v); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return DefaultBaseURL
}

func (c Config) resolvedKitServiceURL(allowEnvService bool) string {
	var candidates []string
	if allowEnvService {
		candidates = append(candidates, os.Getenv("HEV_KIT_SERVICE"))
	}
	candidates = append(candidates, c.Kit.API, c.Kit.Service)
	for _, v := range candidates {
		if v = strings.TrimSpace(v); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return DefaultKitServiceURL
}

// ResolvedKitServiceURL resolves HEV_KIT_SERVICE (when HEV_API_KEY is set,
// matching internal/attention/identity.go), then [kit].api, then
// [kit].service, then DefaultKitServiceURL, trimmed of trailing slashes.
func (c Config) ResolvedKitServiceURL() string {
	return c.resolvedKitServiceURL(strings.TrimSpace(os.Getenv("HEV_API_KEY")) != "")
}

func (c Config) resolvedHostedBaseURL(allowEnvService bool) string {
	for _, v := range []string{os.Getenv("HEV_FOH_BASE_URL"), c.BaseURL} {
		if v = strings.TrimSpace(v); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return c.resolvedKitServiceURL(allowEnvService) + "/v1/inference"
}

// ResolvedHostedBaseURL resolves the Club Kit hosted inference endpoint:
// explicit HEV_FOH_BASE_URL, then explicit [foh].base_url, then
// <ResolvedKitServiceURL>/v1/inference.
func (c Config) ResolvedHostedBaseURL() string {
	return c.resolvedHostedBaseURL(strings.TrimSpace(os.Getenv("HEV_API_KEY")) != "")
}

func isHostedInferenceKey(key string) bool {
	k := strings.TrimSpace(key)
	return strings.HasPrefix(k, "hev_") || strings.HasPrefix(k, "hvl_")
}

func isLegacyKitKey(key string) bool {
	return strings.HasPrefix(strings.TrimSpace(key), "hk_")
}

func isKitScopedKey(key string) bool {
	return isHostedInferenceKey(key) || isLegacyKitKey(key)
}

func isOpenRouterHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return h == "openrouter.ai" || strings.HasSuffix(h, ".openrouter.ai")
}

func isLoopbackHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func validateHostedBaseURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return errors.New("club kit inference: invalid base URL")
	}
	host := u.Hostname()
	if isOpenRouterHost(host) {
		return ErrOpenRouterKeyMismatch
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(host) {
			return nil
		}
		return errors.New("club kit inference: plain http:// is only allowed for loopback hosts (localhost, 127.0.0.1, [::1]); use https://")
	default:
		return fmt.Errorf("club kit inference: unsupported URL scheme %q (use https://)", u.Scheme)
	}
}

func validateClientTarget(baseURL, key string) error {
	key = strings.TrimSpace(key)
	if isLegacyKitKey(key) {
		return ErrLegacyKitKey
	}
	if isHostedInferenceKey(key) {
		return validateHostedBaseURL(baseURL)
	}
	return nil
}

// ResolveModel picks the active model: flag, HEV_FOH_MODEL, [foh].model, the
// first favorite, then DefaultModel. available, when set, filters the
// implicit choices (favorites and the default) to providers that are
// configured; fallback is used when the default's provider isn't.
func (c Config) ResolveModel(flag string, favorites []string, available func(id string) bool, fallback string) string {
	for _, v := range []string{flag, os.Getenv("HEV_FOH_MODEL"), c.Model} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	ok := func(id string) bool { return available == nil || available(id) }
	for _, id := range favorites {
		if ok(id) {
			return id
		}
	}
	if !ok(DefaultModel) && fallback != "" {
		return fallback
	}
	return DefaultModel
}

// ResolveSecretRef reads a secret reference through hev secrets. Tests replace it.
var ResolveSecretRef = func(ctx context.Context, ref string) (string, error) {
	return resolveSecretRef(ctx, ref, time.Hour)
}

func resolveSecretRef(ctx context.Context, ref string, ttl time.Duration) (string, error) {
	uri, err := secrets.ExpandRef(ref)
	if err != nil {
		return "", err
	}
	refs := []secrets.Ref{{Name: "FOH_API_KEY", URI: uri, TTL: ttl}}
	values, err := secrets.ResolveWith(ctx, refs, secrets.Options{Caller: secrets.DefaultCaller("hev foh"), Notice: io.Discard})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(values["FOH_API_KEY"]), nil
}

// ErrNoAPIKey is returned when no inference API key is configured.
var ErrNoAPIKey = errors.New("no inference API key: set OPENROUTER_API_KEY, HEV_API_KEY, [foh].api_key_ref, or sign in with `hev login`")

// ErrNoProviders is returned when neither OpenRouter/Club Kit nor a Codex
// ChatGPT subscription login is available.
var ErrNoProviders = errors.New("no model provider: set OPENROUTER_API_KEY (or HEV_API_KEY, [foh].api_key_ref, or `hev login`), or sign in to Codex with a ChatGPT plan (`codex login`)")

// ErrLegacyKitKey is returned when a legacy hk_ Club Kit key is resolved for
// inference; legacy hk_ keys do not authorize hosted inference.
var ErrLegacyKitKey = errors.New("legacy hk_ Club Kit keys do not authorize hosted inference: run `hev login` to obtain a hev_ key, set OPENROUTER_API_KEY, or sign in to Codex (`codex login`)")

// ImplicitLegacyKitKeyError wraps ErrLegacyKitKey when the legacy hk_ key came
// from the implicit [kit].key sign-in fallback rather than an explicit
// environment variable or [foh].api_key_ref. Callers that support fallback
// providers (such as codex-sub) can warn and continue, while surfacing
// ErrLegacyKitKey if no other provider resolves.
type ImplicitLegacyKitKeyError struct{}

func (ImplicitLegacyKitKeyError) Error() string { return ErrLegacyKitKey.Error() }
func (ImplicitLegacyKitKeyError) Unwrap() error { return ErrLegacyKitKey }

// ErrOpenRouterKeyMismatch is returned when a Club Kit or Layer key (hev_,
// hvl_, or hk_) would be sent to openrouter.ai.
var ErrOpenRouterKeyMismatch = errors.New("refusing to send Club Kit/Layer key (hev_/hvl_/hk_) to openrouter.ai: use Club Kit hosted /v1/inference or set OPENROUTER_API_KEY")

type keySource int

const (
	keySourceNone keySource = iota
	keySourceOpenRouterEnv
	keySourceHevEnv
	keySourceAPIKeyRef
	keySourceKitConfig
)

func (c Config) resolveKeySource(ctx context.Context) (string, keySource, error) {
	if v := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); v != "" {
		return v, keySourceOpenRouterEnv, nil
	}
	if v := strings.TrimSpace(os.Getenv("HEV_API_KEY")); v != "" {
		return v, keySourceHevEnv, nil
	}
	if ref := strings.TrimSpace(c.APIKeyRef); ref != "" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		v, err := ResolveSecretRef(ctx, ref)
		if err != nil {
			return "", keySourceNone, fmt.Errorf("read [foh].api_key_ref: %s", Scrub(err.Error(), v))
		}
		if v == "" {
			return "", keySourceNone, errors.New("read [foh].api_key_ref: empty")
		}
		return v, keySourceAPIKeyRef, nil
	}
	if v := strings.TrimSpace(c.Kit.Key); v != "" && isKitScopedKey(v) {
		return v, keySourceKitConfig, nil
	}
	return "", keySourceNone, ErrNoAPIKey
}

// ResolveAPIKey reads OPENROUTER_API_KEY, HEV_API_KEY, [foh].api_key_ref, then
// [kit].key (when [foh].api_key_ref is unset). Legacy hk_ keys fail closed.
// The returned error never includes the key.
func (c Config) ResolveAPIKey(ctx context.Context) (string, error) {
	key, src, err := c.resolveKeySource(ctx)
	if err != nil {
		return "", err
	}
	if isLegacyKitKey(key) {
		if src == keySourceKitConfig {
			return "", ImplicitLegacyKitKeyError{}
		}
		return "", ErrLegacyKitKey
	}
	return key, nil
}

// ResolveEndpoint resolves the inference credential and validated target base
// URL. Direct OpenRouter keys target OpenRouter; hev_/hvl_ keys route to Club
// Kit hosted /v1/inference with HTTPS/loopback enforcement and a hard guard
// refusing openrouter.ai; legacy hk_ keys fail closed before any network call.
func (c Config) ResolveEndpoint(ctx context.Context) (InferenceEndpoint, error) {
	key, src, err := c.resolveKeySource(ctx)
	if err != nil {
		return InferenceEndpoint{}, err
	}
	if isLegacyKitKey(key) {
		if src == keySourceKitConfig {
			return InferenceEndpoint{}, ImplicitLegacyKitKeyError{}
		}
		return InferenceEndpoint{}, ErrLegacyKitKey
	}
	if isHostedInferenceKey(key) {
		var baseURL string
		switch src {
		case keySourceOpenRouterEnv:
			baseURL = c.ResolvedBaseURL()
		case keySourceHevEnv:
			baseURL = c.resolvedHostedBaseURL(true)
		default:
			baseURL = c.resolvedHostedBaseURL(false)
		}
		if err := validateHostedBaseURL(baseURL); err != nil {
			return InferenceEndpoint{}, err
		}
		return InferenceEndpoint{Mode: InferenceModeClubKit, BaseURL: baseURL, APIKey: key}, nil
	}
	return InferenceEndpoint{Mode: InferenceModeOpenRouter, BaseURL: c.ResolvedBaseURL(), APIKey: key}, nil
}

// ResolvedDocsURL returns the documentation base URL from HEV_DOCS_URL,
// [foh].docs_url, or DefaultDocsURL.
func (c Config) ResolvedDocsURL() string {
	if v := strings.TrimSpace(os.Getenv("HEV_DOCS_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(c.DocsURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	return strings.TrimRight(DefaultDocsURL, "/")
}

// DocsLLMsURL derives the /llms.txt index URL for the configured docs URL.
func (c Config) DocsLLMsURL() string {
	u := c.ResolvedDocsURL()
	if strings.HasSuffix(u, "/docs") {
		return strings.TrimSuffix(u, "/docs") + "/llms.txt"
	}
	return u + "/llms.txt"
}

// secretEnv lists variables kept out of tool subprocesses.
var secretEnv = []string{"OPENROUTER_API_KEY", "HEV_API_KEY", "HEVLAYER_API_KEY"}
