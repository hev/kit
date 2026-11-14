package dashboard

import (
	"net/http"

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
