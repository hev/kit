// Package dashboard exposes kit's read-side UI to private hosted wrappers.
package dashboard

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/serve"
)

// Link is a wrapper page in the dashboard's top bar: same-origin ("/team")
// or absolute https.
type Link = serve.Link

// Credentials belong to the authenticated tester. Namespace is the archive
// base (for example kit-alice-traces), without -sessions/-blocks/-evals.
type Credentials struct {
	Key       string
	Namespace string
}

// Resolver verifies the wrapper's session and returns that tester's scoped
// gateway key. Errors and incomplete credentials fail closed with HTTP 401.
// It must be safe for concurrent calls. Never resolve browser-supplied tenant
// IDs or gateway keys without verifying the private session binding.
type Resolver func(*http.Request) (Credentials, error)

// Config contains deployment settings, never a fallback gateway credential.
type Config struct {
	Endpoint string
	Store    string
	// Transport may share connection pools, but must not inject credentials.
	// Redirects are refused and gateway calls inherit the browser request context.
	Transport http.RoundTripper
	Timeout   time.Duration
	// Links are the wrapper's own pages, shown in the dashboard's top bar.
	Links []Link
}

// NewHosted serves all dashboard routes behind resolve, including HTML/assets.
// Each request gets its own Layer client and archive/eval/baseline caches.
// Local filesystem factory accounting is deliberately not configured.
func NewHosted(cfg Config, resolve Resolver) (http.Handler, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("dashboard requires an explicit HTTP(S) gateway endpoint")
	}
	if resolve == nil {
		return nil, errors.New("dashboard requires a session resolver")
	}
	// Validate deployment capability settings before accepting requests.
	if _, err := layer.New(cfg.Endpoint, "", "", "").WithStore(cfg.Store); err != nil {
		return nil, err
	}
	transport := cfg.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		credentials, err := resolve(r)
		if err != nil || strings.TrimSpace(credentials.Key) == "" || !validNamespace(credentials.Namespace) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		client, _ := layer.New(cfg.Endpoint, credentials.Key, credentials.Namespace, "").WithStore(cfg.Store)
		client.HTTP = &http.Client{Transport: requestTransport{r.Context(), transport}, Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		serve.New(client).WithCacheTTL(0).WithLinks(cfg.Links...).Handler().ServeHTTP(w, r)
	}), nil
}

func validNamespace(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

type requestTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t requestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(r.Clone(t.ctx))
}
