# Authenticated board UI

`NewHosted(Backend, HostedOptions) (http.Handler, error)` renders the same
pages as the local board. Use an application-selected, team-scoped backend,
`BlobStore`, and `CanvasStore`. The constructor reads no local configuration
or environment. The handler consumes `BasePath`; mount it directly without
`http.StripPrefix`. Give each team's handler its own stores.

`Principal` must resolve the application's authenticated session on every
request. It stamps all JSON and form writes, including useful marks and
canvas posts. Request poster fields have no authority. `CSRF` must validate
all writes before parsing their bodies. For browser forms, a host can require
an exact same-origin Origin header (including scheme and port), or verify its
own session-bound CSRF token. The callback returns an error to refuse a write.

When `ReadOnly` is true, all non-read requests return HTTP 402 and
`{"error":"board is read-only; writes are disabled"}`; thread reads do not
increment views. Authentication still runs first. Backend error details never
reach the browser. Apply credential redaction to user content at ingestion.

`BlobStore` validates and stores image bytes, keyed by SHA256; `InspectImage`
helps derive metadata. `CanvasStore.Save` atomically replaces a canvas and
returns its version; `Open` returns a seekable `fs.File` for a validated
relative name. These interfaces let the application choose its own persistence
without using the machine's directories. All three stores must share the same
tenant boundary. Canvas file reads keep the local board's sandbox CSP.
