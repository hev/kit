package foh

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Provider IDs. A model's provider is stamped on every turn of a session
// trace next to the model.
const (
	ProviderOpenRouter      = "openrouter"
	ProviderCodexSub        = "codex-sub"
	ProviderScopedInference = "club-kit-scoped"
)

// CodexModelPrefix marks a model served by the codex-sub provider. The rest
// of the ID is the upstream Codex model name.
const CodexModelPrefix = "codex/"

// Provider is one model backend: it streams chat turns with tool calls and
// lists the models it serves.
type Provider interface {
	// ID names the provider (ProviderOpenRouter, ProviderCodexSub).
	ID() string
	// Stream runs one request and assembles the streamed reply, calling
	// onDelta for each text fragment and each tool call start.
	Stream(ctx context.Context, req ChatRequest, onDelta func(Delta)) (ChatResult, error)
	// Models lists the catalog. err may report a failed refresh even when a
	// fallback list is returned.
	Models(ctx context.Context, refresh bool) ([]Model, CatalogSource, error)
}

// secretHolder is a Provider that holds credentials to scrub from errors.
type secretHolder interface{ Secrets() []string }

// ProviderOf names the provider that serves model id: codex/... is
// codex-sub, everything else is OpenRouter.
func ProviderOf(id string) string {
	if strings.HasPrefix(strings.TrimSpace(id), scopedModelPrefix) {
		return ProviderScopedInference
	}
	if strings.HasPrefix(strings.TrimSpace(id), CodexModelPrefix) {
		return ProviderCodexSub
	}
	return ProviderOpenRouter
}

// ProviderID is m.Provider, or the provider its ID implies.
func (m Model) ProviderID() string {
	if m.Provider != "" {
		return m.Provider
	}
	return ProviderOf(m.ID)
}

// Badge is the picker's provider/cost tag: "[codex-sub] sub" for a
// subscription model, "[OR] $3.00/$15.00" for OpenRouter.
func (m Model) Badge() string {
	switch p := m.ProviderID(); p {
	case ProviderCodexSub:
		return "[codex-sub] sub"
	case ProviderOpenRouter:
		return "[OR] " + m.PriceLabel()
	default:
		return "[" + p + "] " + m.PriceLabel()
	}
}

// ErrNoProvider is returned when a model's provider isn't configured.
var ErrNoProvider = errors.New("provider not available")

// Router is the Provider the agent loop talks to: it sends each request to
// the provider that serves the request's model, so the model can move across
// providers mid-thread while the history stays with the agent.
type Router struct {
	providers []Provider
}

// NewRouter routes across ps; nil entries are skipped. The first provider's
// catalog source is the merged source.
func NewRouter(ps ...Provider) *Router {
	r := &Router{}
	for _, p := range ps {
		if p != nil {
			r.providers = append(r.providers, p)
		}
	}
	return r
}

// ID is "router".
func (r *Router) ID() string { return "router" }

// Has reports whether provider id is configured.
func (r *Router) Has(id string) bool { return r.get(id) != nil }

// Providers lists the configured provider IDs.
func (r *Router) Providers() []string {
	var out []string
	for _, p := range r.providers {
		out = append(out, p.ID())
	}
	return out
}

// Provider returns the configured provider with id, or nil.
func (r *Router) Provider(id string) Provider { return r.get(id) }

func (r *Router) get(id string) Provider {
	for _, p := range r.providers {
		if p.ID() == id {
			return p
		}
	}
	return nil
}

// Resolve returns the provider that serves model.
func (r *Router) Resolve(model string) (Provider, error) {
	id := ProviderOf(model)
	if p := r.get(id); p != nil {
		if scoped, ok := p.(*scopedInference); ok {
			if _, _, err := scoped.scope(model); err != nil {
				return nil, err
			}
		}
		return p, nil
	}
	if r.Has(ProviderScopedInference) {
		return nil, ErrScopeSelection
	}
	switch id {
	case ProviderScopedInference:
		return nil, fmt.Errorf("%w: configure [[foh.key_scopes]]", ErrNoProvider)
	case ProviderCodexSub:
		return nil, fmt.Errorf("%s: %w: no ChatGPT subscription login found for Codex (run `codex login`)", id, ErrNoProvider)
	default:
		return nil, fmt.Errorf("%s: %w: %v", id, ErrNoProvider, ErrNoAPIKey)
	}
}

// Stream routes req to its model's provider.
func (r *Router) Stream(ctx context.Context, req ChatRequest, onDelta func(Delta)) (ChatResult, error) {
	p, err := r.Resolve(req.Model)
	if err != nil {
		return ChatResult{}, err
	}
	return p.Stream(ctx, req, onDelta)
}

// Models merges every provider's catalog: subscription models first, then
// OpenRouter. The source is OpenRouter's when it is configured.
func (r *Router) Models(ctx context.Context, refresh bool) ([]Model, CatalogSource, error) {
	var out []Model
	var src CatalogSource
	var errs []error
	seen := map[string]bool{}
	// Subscription catalogs are short and come first.
	var ordered []Provider
	for _, p := range r.providers {
		if p.ID() == ProviderCodexSub {
			ordered = append(ordered, p)
		}
	}
	for _, p := range r.providers {
		if p.ID() != ProviderCodexSub {
			ordered = append(ordered, p)
		}
	}
	for _, p := range ordered {
		models, s, err := p.Models(ctx, refresh)
		if err != nil {
			errs = append(errs, err)
		}
		if src == "" || p.ID() == ProviderOpenRouter {
			src = s
		}
		for _, m := range models {
			if m.Provider == "" {
				m.Provider = p.ID()
			}
			if m.ID == "" || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			out = append(out, m)
		}
	}
	return out, src, errors.Join(errs...)
}

// Secrets gathers every provider's credentials for scrubbing.
func (r *Router) Secrets() []string {
	var out []string
	for _, p := range r.providers {
		if s, ok := p.(secretHolder); ok {
			out = append(out, s.Secrets()...)
		}
	}
	return out
}

// providerSecrets returns p's credentials, if it holds any.
func providerSecrets(p Provider) []string {
	if s, ok := p.(secretHolder); ok {
		return s.Secrets()
	}
	return nil
}

// providerFor names the provider that serves model under p.
func providerFor(p Provider, model string) string {
	if r, ok := p.(*Router); ok {
		if rp, err := r.Resolve(model); err == nil {
			return rp.ID()
		}
		return ProviderOf(model)
	}
	if p == nil {
		return ""
	}
	return p.ID()
}
