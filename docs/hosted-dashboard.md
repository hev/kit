# Hosted dashboard integration

A private Go service can import `github.com/hev/kit/pkg/dashboard` and mount
`NewHosted(Config, Resolver)` as an `http.Handler`. It serves the same embedded
UI and read APIs as local `hev serve`. The local command is unchanged.

```go
handler, err := dashboard.NewHosted(dashboard.Config{
    Endpoint: gatewayEndpoint, // explicit trusted deployment URL; use HTTPS in production
    Store: "turbopuffer",     // deployment store capability selection
}, func(r *http.Request) (dashboard.Credentials, error) {
    session, err := sessions.Verify(r.Context(), r) // private wrapper implementation
    if err != nil {
        return dashboard.Credentials{}, err
    }
    return dashboard.Credentials{
        Key: session.ScopedGatewayKey,
        Namespace: session.ArchiveNamespace, // e.g. kit-alice-traces
    }, nil
})
if err != nil {
    return err
}
mux.Handle("/", handler)
```

The example's `sessions.Verify` is supplied by layer-pro. The resolver must be
safe for concurrent use and verify the session on every request. Resolver
errors, empty keys, and invalid namespace bases produce a generic 401 before
any gateway call. No environment variable, CLI config, incoming Authorization
header, or query parameter supplies fallback credentials. The handler never
selects the tester from a browser-supplied namespace. There is no admin-key
field in its configuration.

The private layer-pro wrapper owns `/join/session` consumption, atomic one-time
link exchange, expiry/reuse rejection, session storage and revocation, the
session-to-key mapping, secure cookies, container startup, and TLS/DNS. Mount
its exchange route separately before the dashboard catch-all. Avoid logging
session tokens or gateway keys. This package implements none of those private
control-plane operations.

Each request constructs a Layer client with only the resolved key and namespace.
Session lists, session detail and blocks, stats, filter values/facets, transcript
search, eval joins, eval search and baseline reads all use that client. Gateway
namespace suffixes are `-sessions`, `-blocks`, and `-evals`; transcript search
uses the archive base itself. The gateway must authorize the scoped key for
these namespaces. Cross-tenant enforcement remains the gateway's responsibility.

Archive, eval, and baseline state is fresh per request. Hosted mode disables
cross-request archive caching and background warming; it does not retain keys
in a shared server or cache. Authenticated responses carry `Cache-Control:
private, no-store`. Local filesystem factory accounting is unavailable (503).
HTML and `/ui/` assets also require a session. There is no streaming/SSE or
WebSocket dashboard route in the current implementation; future routes under
this handler inherit its resolver boundary and must retain request isolation.

Gateway calls inherit request cancellation and have a three-minute timeout by
default. `Config.Timeout` can override it. `Config.Transport` can share a
connection pool; it must not inject credentials or maintain tenant state.
Redirects are refused to prevent moving a tester credential to another endpoint.
The configured endpoint is trusted deployment input, never resolver/browser input.

Validate the public boundary with `go test -race ./pkg/dashboard`. The tests
exercise sequential and concurrent tenants with identical session IDs, verify
outbound key/namespace pairs across every gateway namespace, check list/detail,
values/stats/eval reads, and prove tenant A finds no result for tenant B's phrase.
They use a simulated scoped gateway; private link lifecycle, live gateway
isolation and hosted TLS require layer-pro acceptance checks.

### Explicit phrase search

`GET /api/search?q=quartz%20amber&mode=phrase&window=all` selects
contiguous phrase matching. Omit `mode`, or use `mode=hybrid`, for the existing
ANN Embed + BM25/RRF retrieval described in RFC0003/RFC0004. Hybrid can return
an own-tenant semantic match without any literal query words. Such a result
alone does not demonstrate tenant leakage. Unknown modes return HTTP 400.

Phrase mode uses the store's existing `ordered_scan` primitive (`rank_by:
["id","asc"]`, filtered `id Gt` pagination). It never issues ANN, Embed,
BM25, HybridText or a fallback hybrid request. The capability must be `supported`; `approximate`, `unsupported` and
`undeclared` ordered scans are refused. Both supported stores
(Turbopuffer and pgvector) declare supported ordered scans; no native phrase operator or
full-text index is required. A reader lacking phrase support returns 503;
a store capability refusal or upstream scan error returns 502, with no fallback.

The matcher applies Go `strings.ToLower` and collapses Unicode whitespace
runs to one ASCII space in both query and matched `text`. It preserves accents
and punctuation, and tests a contiguous substring without word boundaries.
It does not combine chunks, transcript rows or evaluator rows. Reversed words
and words separated by other text do not match. Evaluator matching uses `text`
on the latest eval rows selected by the existing dashboard contract.

Session/window/facet filters still choose eligible sessions, and each scan
retains the transcript `session_id In` or latest-eval `id In` predicate. The
request-scoped hosted credential and namespace remain the tenant boundary.
Matching happens before best-hit selection, session deduplication and `top`
(default 50, valid 1–10000). Phrase hits carry score 0, with session-ID order
and first ID-ordered matching transcript row preferred over equal-score evals.

Phrase mode inspects at most 10,000 filtered rows **per source namespace**,
in ascending ID order, in pages of at most 1,000. One extra row is fetched
only to detect overflow. This is a row budget, independent of `top`, not a
claim of exhaustive archive search. The response echoes `mode`, exposes
`candidate_limit: 10000` and `candidate_truncated: {transcript: bool, eval:
bool}`. If either flag is true, omitted rows may contain matches; the UI
shows that results are incomplete. Concurrent archive writes are not a
snapshot across scan pages. Hybrid retains its existing candidate budget.

The shared dashboard provides a **Hybrid / Exact phrase** selector. Its
`mode` persists in the URL through searches, facet and timeframe changes,
reloads and shared links. Clearing all filters resets to Hybrid.
