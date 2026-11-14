package dashboard

import (
	"net/http"
	"strings"

	"github.com/hev/kit/internal/serve"
)

// Reader is the least a dashboard store implements: list sessions and read
// one session's blocks. A store that also implements SearchHits and
// PhraseHits (pkg/archive's stores do) gets /api/search.
type Reader = serve.Reader

// ReaderOptions configures NewReaderHandler.
type ReaderOptions struct {
	Links []Link
}

// NewReaderHandler serves the dashboard's read routes over reader:
// /api/sessions, /api/session/{id}, /api/search, /api/stats and /api/values,
// with the same shapes `hev serve` answers, plus the HTML page. It caches
// nothing across requests, so reader may be built per request (for
// example, scoped to the caller's team) and wrapped with http.StripPrefix.
// Local factory accounting and image bytes are not served.
func NewReaderHandler(reader Reader, opts ReaderOptions) http.Handler {
	return serve.New(reader).WithLinks(opts.Links).WithCacheTTL(0).Handler()
}

// NewTracesHandler serves the kit API's read routes over reader, at the
// contract's paths relative to /v1/traces:
//
//	GET /sessions        same as /api/sessions
//	GET /sessions/{id}   same as /api/session/{id}
//	GET /search          same as /api/search (q, mode=phrase|hybrid, ...)
//	GET /stats           same as /api/stats
//	GET /values          same as /api/values
//
// Mount it with http.StripPrefix("/v1/traces", ...). Anything else is 404;
// writes and deletes are the server's own routes.
func NewTracesHandler(reader Reader) http.Handler {
	inner := NewReaderHandler(reader, ReaderOptions{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p, ok := TracesPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path, r2.URL.RawPath = p, ""
		inner.ServeHTTP(w, r2)
	})
}

// TracesPath maps a kit API read path (relative to /v1/traces) to the
// dashboard's /api path.
func TracesPath(p string) (string, bool) {
	switch p {
	case "/sessions", "/search", "/stats", "/values":
		return "/api" + p, true
	}
	if id, ok := strings.CutPrefix(p, "/sessions/"); ok && id != "" && !strings.Contains(id, "/") {
		return "/api/session/" + id, true
	}
	return "", false
}

// APIPath is TracesPath's inverse: the kit API path (relative to
// /v1/traces) for a dashboard /api read path, so a local dashboard can be
// served from the kit API.
func APIPath(p string) (string, bool) {
	switch p {
	case "/api/sessions", "/api/search", "/api/stats", "/api/values":
		return strings.TrimPrefix(p, "/api"), true
	}
	if id, ok := strings.CutPrefix(p, "/api/session/"); ok && id != "" && !strings.Contains(id, "/") {
		return "/sessions/" + id, true
	}
	return "", false
}
