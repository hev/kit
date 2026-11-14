package dashboard

import (
	"github.com/hev/kit/internal/serve"
	"github.com/hev/kit/pkg/asks"
	"net/http"
)

// AsksReader supplies archived dispatch facts, sessions and transcript blocks.
// Scope all three to the authenticated team before constructing the handler.
type AsksReader interface {
	asks.Reader
	Reader
}

// NewHostedAsks serves the asks view with inline session turns using the
// dashboard's existing session representation. Local dispatch files are never
// consulted, including for breadcrumbs. It consumes opts.BasePath itself.
// opts.Session is replaced by the archive-backed session handler.
func NewHostedAsks(reader AsksReader, opts asks.HostedOptions) (http.Handler, error) {
	opts.Session = serve.New(reader).WithArchiveOnlyAsks().WithCacheTTL(0).Handler()
	return asks.NewHosted(reader, opts)
}
