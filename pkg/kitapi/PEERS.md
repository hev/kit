# Peers API contract

All calls use `Authorization: Bearer <kit key>` and JSON under the configured
API origin. The server resolves team and author from that key; no write input
has an author field. It validates presence host and message sender host against
the key's registered stable host, returning 403 on mismatch. Other fields are
asserted observations, not authenticated agent identities. This API records
presence and messages; it does not deliver messages.

All four calls return 200 on success, 401 for revoked/invalid keys, 403 for
forbidden access, 400 for invalid input/query, 429 (optionally `Retry-After` in
seconds or HTTP date), and 5xx for service failures. Errors use
`{"error":"message"}`. Writes return 413 if their JSON body exceeds 1 MiB;
presence also limits each snapshot to 1000 sessions. The existing client retries
transport errors, 429 and 5xx with bounded backoff; it never follows redirects.
The background peers uploader makes one attempt per later send/beacon, outside
local delivery, with a 25-second overall timeout and no inline HTTP retries.

## Presence

`POST /v1/peers/presence` replaces one machine's full session snapshot:

```json
{"host":"laptop","version":"0.7.0","os":"darwin","observed_at":"2026-11-14T12:00:00Z","sessions":[{"name":"desk","harness":"codex","model":"example-model","project":"kit","branch":"main","state":"working","started_at":"2026-11-14T11:00:00Z","last_active_at":"2026-11-14T11:59:59Z"}]}
```

```json
{"ttl_seconds":180,"expires_at":"2026-11-14T12:03:00Z"}
```

`observed_at` is a required client timestamp/revision. The server replaces only
when it is newer than the last revision for that team/host. Replaying an equal
or older revision returns the current expiry without changing anything. A
fresh revision sets `expires_at` to server receipt time plus the returned TTL.
Keep the last revision even after expiry so delayed retries cannot resurrect
old presence. Recommended cadence is 60 seconds; clients do not replay beacons
older than 180 seconds. An empty sessions array clears all sessions. Model and
start/activity timestamps are omitted when unknown; project is a base name,
never a local path. OS uses Go's OS labels. Presence is ephemeral, counts as
zero stored content bytes and **continues working for read-only teams** (no
402). Reads work in that state as well.

`GET /v1/peers/presence` has no body or query. It returns live machines only,
sorted by host, with sessions in snapshot order. At `now >= expires_at` the
machine is absent. It has no paging:

```json
{"ttl_seconds":180,"machines":[{"host":"laptop","version":"0.7.0","os":"darwin","observed_at":"2026-11-14T12:00:00Z","sessions":[{"name":"desk","harness":"codex","project":"kit","branch":"main","state":"working"}],"author_id":"member-1","expires_at":"2026-11-14T12:03:00Z"}]}
```

## Message log

`POST /v1/peers/messages` appends one sender-originated message:

```json
{"id":"123456781234123412341234567890ab","sender":"laptop/desk","recipients":["mini/review","laptop/helper"],"time":"2026-11-14T12:00:00Z","kind":"chat","chat_id":"thread-1","title":"Review","body":"Ready for review"}
```

```json
{"id":"123456781234123412341234567890ab","sender":"laptop/desk","recipients":["mini/review","laptop/helper"],"time":"2026-11-14T12:00:00Z","kind":"chat","chat_id":"thread-1","title":"Review","body":"Ready for review","author_id":"member-1","stored_bytes":217}
```

The example's `stored_bytes` is calculated from the compact Go JSON encoding
of its request (`PeerStoredBytes`). The server must calculate this value rather
than trust any client-supplied count.

The idempotency key is a **client-supplied full UUID**, opaque to the server,
unique per team. Both 32 hex and canonical hyphenated representations are
accepted; keys are exact strings, not normalized. An identical repeated append
returns the original row without storing or billing it again. Reusing the same
key with different request content returns **409** and never overwrites the
original. An existing identical entry may be returned even during read-only
status. A new append to a read-only team returns **402**
`{"error":"billing_read_only"}`; clients retain the entry and retry later.

Addresses are `HOST/NAME` with ASCII letters, digits, underscore, hyphen and
period in each part, plus colon/percent in the session part for pane addresses.
Sender, one or more recipients, nonzero RFC3339 time and nonempty body are
required. Kind is `direct` or `chat`; direct omits chat fields, chat requires
`chat_id` and may include a title. No additional per-field length limit is
assumed beyond the 1 MiB encoded request limit.

Direct sends reuse their existing full ID. Chats have no shared sender message
ID on the existing peer wire: the sending client creates a full hosted-only
UUID when first queued and persists it in a **separate team-bound hosted
outbox entry**, reusing it unchanged on every upload retry, including restart
or lost response. There is one hosted row for the sender's chat, with its
recipients/thread as fields, not one row per recipient delivery. No ID is added
to peer wire formats, `_chat`, relayed chats or Python-compatible state files.

Stored content bytes for a unique message are the UTF-8 length of compact
`json.Marshal(PeerMessageIn)` with the Go struct field order shown above (Go's
standard HTML-safe string escaping, no whitespace or trailing newline, absent
optional fields omitted). Count this once on insert, after sender redaction,
including ID, addresses, time, kind, chat fields and body. Exclude author ID,
stored_bytes itself, ingestion/index metadata, retries and presence. The
exported `PeerStoredBytes` is the reference implementation. A hosted service
can use a different internal representation while preserving this count.

`GET /v1/peers/messages?limit=50&machine=laptop&session=laptop%2Fdesk&chat=thread-1`
has no body. Every filter is optional and filters combine with AND. Machine
matches a sender or recipient host; session is an exact `HOST/NAME` sender or
recipient; chat matches `chat_id`. Limit defaults to 50, allowed 1..200.

```json
{"messages":[{"id":"123456781234123412341234567890ab","sender":"laptop/desk","recipients":["mini/review","laptop/helper"],"time":"2026-11-14T12:00:00Z","kind":"chat","chat_id":"thread-1","title":"Review","body":"Ready for review","author_id":"member-1","stored_bytes":217}],"next_cursor":"OPAQUE"}
```

Newest client time first, ties by ID descending. `cursor` is an opaque,
team-and-filter-bound exclusive position after the previous page's last row.
Preserve filters when following it (limit may change). Inserts ahead of it do
not shift later pages. Invalid, cross-team or cross-filter cursors return 400.
No `next_cursor` means end. Empty results are `{"messages":[]}`.

## Hosted view

`pkg/peersview.NewHosted` accepts a team-scoped `Reader` with
`PeerPresence(context.Context) (kitapi.PresencePage, error)` and
`PeerMessages(context.Context, url.Values) (kitapi.PeerMessagesPage, error)`,
a base path and a host-supplied principal resolver. Mount without StripPrefix:

```go
h, err := peersview.NewHosted(teamReader, peersview.HostedOptions{
    BasePath: "/team/peers",
    Principal: func(r *http.Request) (peersview.Principal, error) {
        id, err := authenticatedViewerID(r) // supplied by the host
        return peersview.Principal{ID: id}, err
    },
})
if err != nil { return err }
mux.Handle("/team/peers", h)
mux.Handle("/team/peers/", h)
```

The host must scope the reader to the viewer's team before constructing the
handler. Both panes render with empty data, use escaped templates, expose no
write route, local store, path configuration or connection details, and return
private no-store responses with a restrictive CSP. Stored text is plain text;
there are no message-body links. Filter forms use GET. Reader failures return
generic errors. Project paths are reduced to base names, and common local
paths, loopback URLs and kit keys in displayed text are suppressed.
