package foh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Model is one entry of the merged model catalog.
type Model struct {
	ID string `json:"id"`
	// Provider is the provider ID that serves the model; empty means the
	// provider ProviderOf(ID) derives.
	Provider      string `json:"provider,omitempty"`
	Name          string `json:"name,omitempty"`
	ContextLength int    `json:"context_length,omitempty"`
	// PromptPrice and CompletionPrice are USD per million tokens; negative
	// means unknown.
	PromptPrice     float64 `json:"prompt_price"`
	CompletionPrice float64 `json:"completion_price"`
}

// ContextLabel renders the context window compactly ("200k", "1M").
func (m Model) ContextLabel() string {
	n := m.ContextLength
	switch {
	case n <= 0:
		return "?"
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1_000_000), ".0") + "M"
	case n >= 1000:
		return fmt.Sprintf("%dk", (n+500)/1000)
	default:
		return strconv.Itoa(n)
	}
}

// PriceLabel renders prompt/completion $ per million tokens.
func (m Model) PriceLabel() string {
	p := func(v float64) string {
		if v < 0 {
			return "?"
		}
		if v == 0 {
			return "free"
		}
		return fmt.Sprintf("$%.2f", v)
	}
	return p(m.PromptPrice) + "/" + p(m.CompletionPrice)
}

const (
	// CatalogTTL is how long a fetched catalog is used before refetching.
	CatalogTTL = 24 * time.Hour
	// MaxCatalogModels bounds the cached catalog.
	MaxCatalogModels = 2000
	maxCatalogBytes  = 16 << 20
	catalogTimeout   = 20 * time.Second
)

// StarterModels is the offline fallback when no catalog was ever fetched.
// Slugs may age; the live catalog replaces them on the first fetch.
var StarterModels = []Model{
	{ID: "anthropic/claude-sonnet-5.5", Name: "Anthropic: Claude Sonnet 5.5", ContextLength: 200000, PromptPrice: -1, CompletionPrice: -1},
	{ID: "anthropic/claude-opus-5.5", Name: "Anthropic: Claude Opus 5.5", ContextLength: 200000, PromptPrice: -1, CompletionPrice: -1},
	{ID: "anthropic/claude-haiku-5.5", Name: "Anthropic: Claude Haiku 5.5", ContextLength: 200000, PromptPrice: -1, CompletionPrice: -1},
	{ID: "openai/gpt-5", Name: "OpenAI: GPT-5", ContextLength: 400000, PromptPrice: -1, CompletionPrice: -1},
	{ID: "google/gemini-2.5-pro", Name: "Google: Gemini 2.5 Pro", ContextLength: 1048576, PromptPrice: -1, CompletionPrice: -1},
	{ID: "deepseek/deepseek-chat", Name: "DeepSeek: DeepSeek V3", ContextLength: 163840, PromptPrice: -1, CompletionPrice: -1},
}

// CatalogSource says where a catalog came from.
type CatalogSource string

const (
	SourceCache   CatalogSource = "cache"
	SourceNetwork CatalogSource = "network"
	SourceStale   CatalogSource = "stale-cache"
	SourceStarter CatalogSource = "starter"
)

type catalogFile struct {
	FetchedAt time.Time `json:"fetched_at"`
	Models    []Model   `json:"models"`
}

// CatalogPath is ~/.hev/cache/openrouter-models.json.
func CatalogPath() string {
	return filepath.Join(homeDir(), ".hev", "cache", "openrouter-models.json")
}

// Catalog fetches and caches the OpenRouter model list.
type Catalog struct {
	Client *Client
	Path   string
	Now    func() time.Time
}

func (c *Catalog) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Catalog) path() string {
	if c.Path != "" {
		return c.Path
	}
	return CatalogPath()
}

func (c *Catalog) readCache() (catalogFile, bool) {
	f, err := os.Open(c.path())
	if err != nil {
		return catalogFile{}, false
	}
	defer f.Close()
	var cf catalogFile
	if json.NewDecoder(io.LimitReader(f, maxCatalogBytes)).Decode(&cf) != nil || len(cf.Models) == 0 {
		return catalogFile{}, false
	}
	return cf, true
}

// Models returns the catalog: a fresh cache, else the network, else a stale
// cache, else StarterModels. err reports a failed fetch even when a fallback
// is returned.
func (c *Catalog) Models(ctx context.Context, refresh bool) ([]Model, CatalogSource, error) {
	cached, ok := c.readCache()
	if ok && !refresh && c.now().Sub(cached.FetchedAt) < CatalogTTL && !cached.FetchedAt.After(c.now()) {
		return cached.Models, SourceCache, nil
	}
	models, err := c.fetch(ctx)
	if err == nil {
		_ = c.writeCache(catalogFile{FetchedAt: c.now(), Models: models})
		return models, SourceNetwork, nil
	}
	if ok {
		return cached.Models, SourceStale, err
	}
	return append([]Model(nil), StarterModels...), SourceStarter, err
}

func (c *Catalog) fetch(ctx context.Context) ([]Model, error) {
	if c.Client == nil {
		return nil, errors.New("openrouter: no client")
	}
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	req, err := c.Client.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.httpClient().Do(req)
	if err != nil {
		return nil, c.Client.scrub(fmt.Errorf("openrouter models: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, c.Client.statusError(resp)
	}
	var raw struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCatalogBytes)).Decode(&raw); err != nil {
		return nil, c.Client.scrub(fmt.Errorf("openrouter models: decode: %w", err))
	}
	var out []Model
	for _, d := range raw.Data {
		if d.ID == "" {
			continue
		}
		out = append(out, Model{ID: d.ID, Name: d.Name, ContextLength: d.ContextLength,
			PromptPrice: perMillion(d.Pricing.Prompt), CompletionPrice: perMillion(d.Pricing.Completion)})
		if len(out) == MaxCatalogModels {
			break
		}
	}
	if len(out) == 0 {
		return nil, errors.New("openrouter models: empty catalog")
	}
	return out, nil
}

func perMillion(perToken string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(perToken), 64)
	if err != nil || v < 0 {
		return -1
	}
	return v * 1e6
}

func (c *Catalog) writeCache(cf catalogFile) error {
	return writeFileAtomic(c.path(), cf)
}

// writeFileAtomic writes v as JSON through a temp file and rename.
func writeFileAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
