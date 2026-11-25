package foh

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/secrets"
)

const (
	scopedModelPrefix   = "scope/"
	maxKeyScopes        = 8
	scopeResolveTimeout = 135 * time.Second
	scopeVerifyTimeout  = 10 * time.Second
)

var (
	ErrScopeSelection  = errors.New("select a configured account and workspace via scope/<name>/<model>")
	scopeNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	hostedTokenPattern = regexp.MustCompile(`^(hev|hvl)_[A-Za-z0-9]{32}$`)
)

// KeyScope binds a local selector to the hosted account and workspace IDs.
// APIKeyRef is resolved independently for each request.
type KeyScope struct {
	Name      string `toml:"name"`
	Account   string `toml:"account"`
	Workspace string `toml:"workspace"`
	APIKeyRef string `toml:"api_key_ref"`
}

type scopedInference struct {
	service string
	scopes  []KeyScope
	resolve func(context.Context, string) (string, error)
}

// ScopedInference routes scoped model selectors to Club Kit. It deliberately
// ignores process-level keys and generic inference endpoint overrides.
func (c Config) ScopedInference() (Provider, error) {
	if len(c.KeyScopes) == 0 || len(c.KeyScopes) > maxKeyScopes {
		return nil, errors.New("configure between one and eight FOH key scopes")
	}
	names := make(map[string]bool)
	pairs := make(map[[2]string]bool)
	refs := make(map[string]bool)
	for _, scope := range c.KeyScopes {
		pair := [2]string{scope.Account, scope.Workspace}
		validRef := secrets.ValidRef(scope.APIKeyRef) || secrets.ValidNeutral(scope.APIKeyRef)
		if !scopeNamePattern.MatchString(scope.Name) || !validScopeID(scope.Account) || !validScopeID(scope.Workspace) || !validRef {
			return nil, errors.New("invalid FOH key scope configuration")
		}
		if names[scope.Name] || pairs[pair] || refs[scope.APIKeyRef] {
			return nil, errors.New("ambiguous FOH key scope configuration")
		}
		names[scope.Name], pairs[pair], refs[scope.APIKeyRef] = true, true, true
	}
	service := c.resolvedKitServiceURL(false)
	u, err := url.Parse(service)
	if err != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || validateHostedBaseURL(service) != nil {
		return nil, errors.New("invalid Club Kit service URL for scoped inference")
	}
	return &scopedInference{
		service: service, scopes: append([]KeyScope(nil), c.KeyScopes...),
		resolve: func(ctx context.Context, ref string) (string, error) {
			return resolveSecretRef(ctx, ref, 0, scopeResolveTimeout)
		},
	}, nil
}

func validScopeID(id string) bool {
	return id != "" && len(id) <= 128 && strings.TrimSpace(id) == id && Scrub(id) == id
}

func (*scopedInference) ID() string { return ProviderScopedInference }

func (p *scopedInference) scope(model string) (KeyScope, string, error) {
	selector, ok := strings.CutPrefix(strings.TrimSpace(model), scopedModelPrefix)
	name, upstream, hasModel := strings.Cut(selector, "/")
	if ok && hasModel && upstream != "" && strings.TrimSpace(upstream) == upstream && Scrub(upstream) == upstream {
		for _, scope := range p.scopes {
			if name == scope.Name {
				return scope, upstream, nil
			}
		}
	}
	return KeyScope{}, "", ErrScopeSelection
}

func (p *scopedInference) client(ctx context.Context, scope KeyScope) (*Client, error) {
	key, err := p.resolve(ctx, scope.APIKeyRef)
	switch {
	case errors.Is(err, context.Canceled):
		return nil, errors.New("Club Kit credential resolution canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return nil, errors.New("timed out resolving a Club Kit credential for the selected scope")
	}
	if err != nil || !hostedTokenPattern.MatchString(key) {
		return nil, errors.New("could not resolve a Club Kit credential for the selected scope")
	}

	ctx, cancel := context.WithTimeout(ctx, scopeVerifyTimeout)
	defer cancel()
	client := NewClient(p.service, key)
	req, err := client.newRequest(ctx, http.MethodGet, "/v1/whoami", nil)
	if err != nil {
		return nil, errors.New("could not verify the selected Club Kit scope")
	}
	resp, err := client.httpClient().Do(req)
	if err != nil {
		return nil, errors.New("could not verify the selected Club Kit scope")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Club Kit refused scope verification")
	}
	var identity struct {
		Author struct {
			ID string `json:"id"`
		} `json:"author"`
		Team struct {
			ID string `json:"id"`
		} `json:"team"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&identity); err != nil ||
		identity.Author.ID != scope.Account || identity.Team.ID != scope.Workspace {
		return nil, errors.New("Club Kit credential does not match the selected account and workspace")
	}
	return NewClient(p.service+"/v1/inference", key), nil
}

func (p *scopedInference) Stream(ctx context.Context, req ChatRequest, onDelta func(Delta)) (ChatResult, error) {
	scope, model, err := p.scope(req.Model)
	if err != nil {
		return ChatResult{}, err
	}
	client, err := p.client(ctx, scope)
	if err != nil {
		return ChatResult{}, err
	}
	req.Model = model
	// Scrub after assembling the response so credentials split across SSE
	// fragments cannot reach terminal output, tool arguments, or session history.
	res, err := client.Stream(ctx, req, nil)
	if err != nil {
		return ChatResult{}, err
	}
	res.Model = scopedModelPrefix + scope.Name + "/" + Scrub(model, client.APIKey)
	res.Content = Scrub(res.Content, client.APIKey)
	res.FinishReason = Scrub(res.FinishReason, client.APIKey)
	for i := range res.ToolCalls {
		call := &res.ToolCalls[i]
		call.ID = Scrub(call.ID, client.APIKey)
		call.Function.Name = Scrub(call.Function.Name, client.APIKey)
		call.Function.Arguments = Scrub(call.Function.Arguments, client.APIKey)
	}
	if onDelta != nil && res.Content != "" {
		onDelta(Delta{Text: res.Content})
	}
	return res, nil
}

func (p *scopedInference) Models(ctx context.Context, _ bool) ([]Model, CatalogSource, error) {
	// Each scope has its own deadlines. A slow or refused scope must leave
	// the sibling catalog available and must never substitute another key.
	catalogs := make([][]Model, len(p.scopes))
	errs := make([]error, len(p.scopes))
	var loads sync.WaitGroup
	for i, scope := range p.scopes {
		loads.Go(func() {
			catalogs[i], errs[i] = p.modelsForScope(ctx, scope)
		})
	}
	loads.Wait()
	var models []Model
	for _, catalog := range catalogs {
		models = append(models, catalog...)
	}
	return models, SourceNetwork, errors.Join(errs...)
}

func (p *scopedInference) modelsForScope(ctx context.Context, scope KeyScope) ([]Model, error) {
	client, err := p.client(ctx, scope)
	if err != nil {
		return nil, err
	}
	// Skip Catalog.Models, which shares a disk cache and offline fallback
	// among the legacy single-key clients.
	catalog, err := (&Catalog{Client: client}).fetch(ctx)
	if err != nil {
		return nil, err
	}
	models := catalog[:0]
	for _, model := range catalog {
		if Scrub(model.ID, client.APIKey) != model.ID {
			continue
		}
		model.ID = scopedModelPrefix + scope.Name + "/" + model.ID
		model.Name = Scrub(model.Name, client.APIKey) + " (" + scope.Name + ")"
		model.Provider = ProviderScopedInference
		models = append(models, model)
	}
	return models, nil
}
