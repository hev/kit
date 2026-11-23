package foh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/secrets"
)

// DefaultBaseURL is OpenRouter's OpenAI-compatible API root.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

// DefaultModel is used when no flag, environment, config or favorite names one.
const DefaultModel = "anthropic/claude-sonnet-5.5"

// DefaultDocsURL is where the harness fetches its own documentation.
// Overridable at build time via -ldflags or at runtime via HEV_DOCS_URL / [foh].docs_url.
var DefaultDocsURL = "https://hevkit.com/docs"

// Config is the optional [foh] table in ~/.hev/config.toml.
type Config struct {
	Model        string `toml:"model"`
	BaseURL      string `toml:"base_url"`
	APIKeyRef    string `toml:"api_key_ref"`
	SystemPrompt string `toml:"system_prompt"`
	DocsURL      string `toml:"docs_url"`
	// Codex is the optional [foh.codex] table for the codex-sub provider.
	Codex CodexConfig `toml:"codex"`
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

// LoadConfig reads [foh] from path (ConfigPath when empty). A missing file or
// table is an empty Config. Errors never carry the path or any value.
func LoadConfig(path string) (Config, error) {
	if path == "" {
		path = ConfigPath()
	}
	var file struct {
		FOH Config `toml:"foh"`
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
		if len(key) > 2 && key[0] == "foh" && key[1] == "codex" {
			return Config{}, fmt.Errorf("unknown field in [foh.codex]: %s", key[2])
		}
		if len(key) > 1 && key[0] == "foh" {
			return Config{}, fmt.Errorf("unknown field in [foh]: %s", key[1])
		}
	}
	return file.FOH, nil
}

// BaseURL resolves HEV_FOH_BASE_URL, then [foh].base_url, then OpenRouter.
func (c Config) ResolvedBaseURL() string {
	for _, v := range []string{os.Getenv("HEV_FOH_BASE_URL"), c.BaseURL} {
		if v = strings.TrimSpace(v); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return DefaultBaseURL
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
	uri, err := secrets.ExpandRef(ref)
	if err != nil {
		return "", err
	}
	refs := []secrets.Ref{{Name: "OPENROUTER_API_KEY", URI: uri, TTL: time.Hour}}
	values, err := secrets.ResolveWith(ctx, refs, secrets.Options{Caller: secrets.DefaultCaller("hev foh"), Notice: io.Discard})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(values["OPENROUTER_API_KEY"]), nil
}

// ErrNoAPIKey is returned when no OpenRouter key is configured.
var ErrNoAPIKey = errors.New("no OpenRouter API key: set OPENROUTER_API_KEY, HEV_API_KEY or [foh].api_key_ref")

// ErrNoProviders is returned when neither OpenRouter nor a Codex ChatGPT
// subscription login is available.
var ErrNoProviders = errors.New("no model provider: set OPENROUTER_API_KEY (or HEV_API_KEY, [foh].api_key_ref), or sign in to Codex with a ChatGPT plan (`codex login`)")

// ResolveAPIKey reads OPENROUTER_API_KEY, HEV_API_KEY, then
// [foh].api_key_ref. The returned error never includes the key.
func (c Config) ResolveAPIKey(ctx context.Context) (string, error) {
	for _, name := range []string{"OPENROUTER_API_KEY", "HEV_API_KEY"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, nil
		}
	}
	if ref := strings.TrimSpace(c.APIKeyRef); ref != "" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		v, err := ResolveSecretRef(ctx, ref)
		if err != nil {
			return "", fmt.Errorf("read [foh].api_key_ref: %s", Scrub(err.Error(), v))
		}
		if v == "" {
			return "", errors.New("read [foh].api_key_ref: empty")
		}
		return v, nil
	}
	return "", ErrNoAPIKey
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
var secretEnv = []string{"OPENROUTER_API_KEY", "HEV_API_KEY"}
