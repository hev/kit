# RFC 0019 — The kit Pro API (`/v1`): folders in OSS, one hosted API in Pro

**Status: Draft.** Spec: [`openapi/kit-v1.yaml`](../../openapi/kit-v1.yaml)
(OpenAPI 3.1.0). Decision: Adam, 2026-11-19, board memo #534. No production
code changes in this RFC.

## Summary

kit has five data surfaces: **Traces**, **Board**, **Orders**, **Peers** and
**Hooks**. In kit OSS each one is a folder on the host and there is no API. In
kit Pro the same five surfaces are served by one hosted REST API under `/v1`,
with one auth scheme, one pagination shape, one error envelope and one search
behaviour. Hooks also push: Pro delivers events to registered webhooks
(OpenAPI 3.1 `webhooks`). Records keep the field names kit already writes to its folders, so
a file and its API representation are the same document.

## Motivation

Today kit's HTTP surface is split across five servers that grew separately:

| Server | Routes | Auth |
|--------|--------|------|
| `hev serve` dashboard (`internal/serve`, `pkg/dashboard`) | `GET /api/sessions`, `/api/session/{id}`, `/api/stats`, `/api/search`, `/api/values`, `/api/image/{sha}`, `/api/factory`, `/api/asks[/{id}]` | none locally; layer-pro session cookie when hosted |
| Board server (`internal/board/server`) | `/api/threads[...]`, `/api/boards`, `/api/blobs`, `/blob/{sha}`, `/api/search`, `/api/stats`, `/api/users/{a}`, `/api/online`, `/api/canvas` | none (read-only flag, CORS list) |
| `hev collect` (`internal/collect`) | `POST /v1/traces` (OTLP), `/v1/app/traces/*`, `/v1/app/boards/{board}/*` (camelCase) | bearer |
| hevd peers listener (`internal/peers/hevd.go`) | `POST /v1/peers/enqueue` | per-host bearer token |
| Dispatch | no HTTP: `hev dispatch rpc` JSON over SSH | SSH |

Two routes are both called `/api/search` and `/api/stats` with different
meanings, board fields are snake_case in one API and camelCase in another, and
orders have no HTTP API at all. A Pro customer cannot be given one key and
one reference. Memo #534 also drops the old API docs pages until this rework
lands, so the spec is what those pages become.

## Decision

1. **The seam is storage, not features.** OSS reads and writes folders. Pro
   reads and writes `/v1`. The CLI and skills keep the same commands; the
   storage mode (`internal/storagemode`) picks the backend, as the
   `layer` / `filesystem` switch already does for the board and trace archive.
2. **One API, five surfaces.** `openapi/kit-v1.yaml` is canonical. Server and
   client code are written against it, not the reverse.
3. **Folder shapes are wire shapes.** `SessionRow`, `BlockRow`, chunk rows,
   board threads and posts, order frontmatter, `attention_event`, steer
   receipts, peer inbox rows and hook events (`hook_event`,
   `peer_message_event`, `board_post_event`) go over the wire with their current JSON
   names. Renames happen only where the API vocabulary differs (below), and
   each is listed.

## Non-goals

- A hosted API for OSS. OSS stays folder-only; `hev serve` and `hev board serve`
  remain local conveniences, not a contract.
- Replacing OTLP. `hev collect`'s `POST /v1/traces` is the OTLP/HTTP receiver
  path that exporters hard-code; it is not part of this API and stays on the
  collector (see Open questions).
- Orchestrator and swarm specs, admission, cluster membership, secrets as
  resources. They ride on Orders and Peers later, additively. Hooks only
  reports admission and cluster changes as events.
- Chat threads (`peers chat`). Messages carry a `thread` id now; thread CRUD
  comes later.

## The seam

| Surface | OSS folder (what kit writes today) | Pro API |
|---------|-----------------------------------|---------|
| Traces | `~/.hev/archive/<namespace>/` — `sessions/`, `blocks/`, `chunks/`, `index.jsonl`, `evals.jsonl`, `blobs/sha256/` (`internal/filearchive`) | `/v1/traces/*` |
| Board | `~/.hev/board/threads/<id>.json`, `blobs/sha256/` (`board.dir`, `HEV_BOARD_DIR`) | `/v1/board/*` |
| Orders | `~/dispatch/<name>/` — `order.md`, `meta.json`, `status.md`, `report.md`, `result.json`, `inbox.md`, `decisions.json`, `attention.json`, `review-notes.json`, `steer/`, `log` | `/v1/orders/*` |
| Peers | `~/.local/state/peers-v2/` (`HEV_PEERS_STATE`) — `hosts/HOST.json`, `inbox/SESSION.jsonl[.receipts.jsonl]`, `sent.jsonl`, `managed/` | `/v1/peers/*` |
| Hooks | `~/.hev/hooks/` — `events.jsonl`, `warnings.log`; handlers are `hev-hook-NAME` executables on PATH | `/v1/hooks/*` and outbound webhooks |

The memo's shorthand paths `~/.hev/traces/` and `~/.hev/peers/` do not exist
today; the table gives the real ones. Whether to move them under `~/.hev/` is
an open question, not part of this API.

### Traces

| Folder | API |
|--------|-----|
| `sessions/<shard>/<key>.json` (one `SessionRow`) | `POST /v1/traces/sessions` (upsert batch), `GET /v1/traces/sessions`, `GET`/`DELETE /v1/traces/sessions/{id}` |
| `blocks/<shard>/<key>.jsonl` (`BlockRow`s) | `POST /v1/traces/blocks`; returned inside session detail |
| `chunks/<shard>/<key>.jsonl` (search rows) | `POST /v1/traces/chunks`; read only through search |
| `evals.jsonl` | returned inside session detail; searched by `/v1/traces/search` |
| `blobs/sha256/…` | `PUT`/`GET /v1/traces/images/{sha256}` |
| `index.jsonl` (newest-first listing) | the list endpoint's ordering |

Ingest is the indexer's existing upsert: the three row kinds the Layer client
writes to `<ns>`, `<ns>-blocks` and `<ns>-sessions` today, minus the
namespace, which the key now resolves. Chunk ids are content-addressed and
session/block ids stable, so retries are idempotent without
`Idempotency-Key`. `DELETE` is `DeleteSessionArchive`: an exact `session_id`
filter on every namespace, verified empty.

Trace rows keep `start`, `end` and `prompt_ts` as epoch milliseconds. They are
stored Layer attributes the dashboard filters on, and converting them would
break round-tripping between a file archive and Pro. Every other timestamp in
the API, and every `since`/`until` filter, is ISO-8601 UTC.

### Board

kit's store calls the unit a *thread* whose messages are *posts* (post #1
opens it). `/v1` uses the vocabulary the order and the CLI use: a **post**
(the thread, numbered `#534`) and its **replies**.

| kit (`internal/board/store`) | `/v1` |
|------------------------------|-------|
| `Thread` + opening `Post` (n = 1) | `BoardPost` (`id` int64, with `body` and `images`) |
| `Post` with n ≥ 2, id `p:<thread>:<n>` | `BoardReply` (same id) |
| `thread_id` | `post_id` |
| `POST /api/threads/{id}/pin`, `/lock` with `{on}` | `PATCH /v1/board/posts/{id}` `{pinned}` / `{locked}` |
| `GET /api/threads?board&tag&author&limit&offset` | `GET /v1/board/posts?board&tag&author&cursor&limit` |
| `GET /api/blobs`, `POST /api/blobs`, `GET /blob/{sha}` | `/v1/board/blobs`, `/v1/board/blobs/{sha256}` (tag *Board blobs*) |

Two behaviours are new in Pro and have no OSS equivalent yet: editing a post
(`title`, `body`, `tags`) and deleting one. Both are limited to the author or
an account admin; post numbers are never reused. Listing puts pinned posts
first on every list. OSS does that only when a `board` filter is set; the
file store should follow once the API ships.

Blobs are included because the board model already supports them cleanly:
content-addressed, write-once, sniffed type (PNG, JPEG, GIF, WebP; no SVG),
10 MB and 8 per message, and every message image must be uploaded first.

### Orders

| kit | `/v1` |
|-----|-------|
| `hev dispatch` (`start`) with an order file | `POST /v1/orders` with `{order}` (the file) or `{spec, body}` |
| `list` | `GET /v1/orders?host&repo&state&orchestrator` |
| `show` (meta, order, status tail, report, result, decisions, attention, review notes) | `GET /v1/orders/{id}` |
| `amend`; `say --resolve KEY`; `rework` / `retry` / `stop` | `PATCH /v1/orders/{id}` with exactly one of `order`, `resolve`, `transition` |
| `stop`; `clean` | `DELETE /v1/orders/{id}` (`?purge=true` for clean) |
| `logs {name, offset}` → `{log, offset, active}` | `GET /v1/orders/{id}/logs?cursor` → `{log, next_cursor, active}` |
| `hev steer` / `say` (note, urgent, halt, redirect) | `POST /v1/orders/{id}/steer` → `SteerMessage` receipt |

The order id is the run name (`^[a-z0-9][a-z0-9-]*$`); orchestrators keep
their `o-<name>` address. `state` is `dispatch.State` exactly (`running`,
`queued`, `halting`, `done`, `blocked`, `needs-review`, `failed`, `timeout`,
`died`, `halted`, `launch-unconfirmed`, plus follow-through's `merged` and
`abandoned`), still derived from the last line of `status.md`. `OrderSpec` is
`schemas/order.schema.json` plus `skills`, which the parser accepts but the
schema has not caught up with; the schema should be fixed in its own change.
Attention notices are `schemas/attention_event.schema.json` verbatim.

### Peers

| kit | `/v1` |
|-----|-------|
| Peer address `HOST/NAME` | id `HOST:NAME` (one path segment), `address` keeps `HOST/NAME` |
| beacon snapshot pane (`hosts/HOST.json`) every 60s, stale after 900s | `PUT /v1/peers/{id}` heartbeat; `stale` after 900s |
| `peers ls --everywhere --json` | `GET /v1/peers` |
| `peers read` | `GET /v1/peers/{id}` (status; `screen` only if the host publishes it) |
| `peers inbox --json` / `sent.jsonl` | `GET /v1/peers/{id}/messages?box=inbox|sent` |
| `peers send`, hevd `POST /v1/peers/enqueue` | `POST /v1/peers/{id}/messages` |
| `peers inbox consume|ack` (`.receipts.jsonl`) | `POST /v1/peers/{id}/receipts` |

ACK tracking is the existing receipt model: a message is held bound to the
recipient's `recipient_incarnation`, moves `queued` → `consumed` (shown to the
agent) → `acknowledged`, only the current incarnation may record receipts, and
an ack needs a consume first. Pro is inbox-only, as hevd's enqueue already is:
nothing is ever pasted into a terminal. The `receipts` route is the one
endpoint beyond the order's list; without it a sender can see `queued` but
the recipient has no way to move it.

### Hooks

OSS hooks are local: `hev hook EVENT --harness claude|codex` normalizes the
harness's hook JSON to `hev.hook.event/v1`, runs `hev-hook-NAME` handlers on
PATH under a 2s budget that always fails open, and appends a redacted copy to
`~/.hev/hooks/events.jsonl` (RFC 0013, `docs/hooks.md`). Pro keeps all of that
and adds webhooks: an HTTPS endpoint registered once for the account receives
the same events from every host.

| kit | `/v1` |
|-----|-------|
| `hev-hook-NAME` on PATH | `POST`/`GET /v1/hooks`, `GET`/`PATCH`/`DELETE /v1/hooks/{id}` |
| stdin to a handler | an outbound delivery (`webhooks.hookEvent`, `webhooks.blockingHookEvent`) |
| handler stdout `{decision, reason, context, system_message}` | the blocking answer `{decision: allow\|block\|halt, reason, add_context, system_message}` |
| `events.jsonl` | `GET /v1/hooks/events` |
| `warnings.log` (handler failures) | `GET /v1/hooks/{id}/deliveries` and `health` on the hook |
| (none) | `POST /v1/hooks/{id}/test`, `POST /v1/hooks/events` |

**Events.** A hook subscribes to exact names, a family (`order-*`) or `*`.
The six harness events are RFC 0013's. Kit already raises five meta-harness
events of its own: `needs-you`, `ready` and `done` (attention notices),
`peer-message` and `board-post`. Pro adds events for changes it can see because
it hosts the other surfaces: `order-created|state|amended|deleted`,
`steer-queued|injected|acked|escalated`, `admission-queued|admitted|refused`,
`cluster-join|suspect|leave`, `peer-online|stale|acked` and
`board-updated|deleted`. None of these exist in kit today; they are the Pro
names, so local handlers can adopt them unchanged later.

**Bodies.** A delivery's body is the document a local handler would read on
stdin, told apart by `schema`: `hev.hook.event/v1` for harness events,
`hev.attention.event/v1`, `hev.peer.message/v1` and `hev.board.post/v1` for
the kit events that exist today, and `hev.hook.event/v1` with `harness: kit`
and a `data` record for the Pro-only ones. That widens
`schemas/hook_event.schema.json`'s `event` and `harness` enums and adds
`host`, `data` and `reason`, all additive. The schema file should follow in its
own change, like `order.schema.json`'s `skills`. Prompt text, tool input,
commands, tool output and message or post bodies go only to hooks registered
with `include_content: true`, and the stored log never has them, the same as
`events.jsonl`.

**Signing.** Every delivery carries `X-Hev-Event`, `X-Hev-Delivery-Id`,
`X-Hev-Hook-Id`, `X-Hev-Timestamp` and `X-Hev-Signature-256: sha256=HEX`, an
HMAC-SHA256 of `TIMESTAMP.BODY` under the hook's secret. Receivers compare in
constant time and reject timestamps more than five minutes old. The secret is
shown once, at creation or rotation. After a rotation the old secret keeps
signing alongside the new one for 24 hours, so the header carries two values.

**Observe-only and blocking.** Observe-only is the default and the only mode
for `stop`, `pre-compact` and every kit event. Any 2xx within 10s is success.
Anything else is retried with backoff (1m, 5m, 30m, 2h, 6h, 15h; seven attempts
over about a day) under the same delivery id. A 410 disables the hook, and so
do 50 straight failures with no success in 72 hours (`disabled_reason`). With
`blocking: true` a hook gates `session-start`, `prompt-submit`, `pre-tool` and
`post-tool`. On a Pro host, `hev hook` sends the event to
`POST /v1/hooks/events`. The server calls the blocking hooks in parallel, each
under its `timeout_ms` (default 1500, below the local 2s budget), and returns
the merged answer: halt beats block beats allow, and `add_context` joins in
hook order. `hev hook` merges that with its local handlers. Blocking
deliveries are never retried and fail open. A timeout, an error, a non-2xx or a
malformed answer counts as `allow` and is recorded as `failed_open`, and if
Pro itself is slow or down the local hook continues. A broken webhook cannot
stop an agent, just as a broken local handler cannot.

## Search: Layer auto-routed

`GET /v1/traces/search` and `GET /v1/board/search` take the same inputs —
`q`, up to seven `also` phrasings (eight in all, `search.MaxPhrasings`), the
surface's filters, `cursor`, `limit` — and return the same `SearchMeta`
(`q`, `also`, `route`, `engine`, `semantic`). The client never chooses a store
or embeds anything. The server routes each query to the best path the
account's store supports and reports it in `route`:

| `route` | When | What runs |
|---------|------|-----------|
| `layer.multi_query` | default | One ANN and one BM25 leg per phrasing, fused by RRF in the store (`pkg/search.Query.Body`) |
| `layer.hybrid_text` | the store has no multi-query | One gateway HybridText expression; the gateway expands and fuses legs |
| `phrase.scan` | traces, `mode=phrase` | Contiguous-text match over an ordered scan (`internal/layer/phrase.go`) |
| `lexical.bm25` | never in Pro | Named so OSS file-store results (`engine` `bm25`/`rg`/`go`) read the same |

Trace search ranks per session: filters select the candidate sessions first,
transcript and eval hits are ranked together, and each session keeps its best
hit. Board search keeps one hit per post and filters on the matching message.
These are today's semantics, unchanged.

## Shared conventions

- **Auth.** `Authorization: Bearer <key>`, the same `HEV_API_KEY` / `[kit] key`
  `hev login` writes and `GET /v1/whoami` already accepts. The key resolves
  the account and its namespaces; no request names a namespace or store, as
  the hosted dashboard already requires. Clients never follow a redirect with
  the key. Read-only keys get 403 on writes.
- **Pagination.** `cursor` (opaque) and `limit` (1–200, default 50) in;
  `{data, next_cursor}` out, `next_cursor` `null` on the last page. Board's
  `offset` and the logs RPC's byte `offset` become cursors.
- **Errors.** `ErrorResponse {error, code, details?, request_id?}`. `error` stays
  the string kit servers return today as `{"error": "..."}`, so existing
  clients keep parsing; `code` is the stable field (`not_found`, `locked`,
  `invalid_order`, …).
- **Time.** ISO-8601 UTC with `Z`, except trace rows' epoch-ms fields. `since`
  also accepts `7d`, `2w` and Go durations, as `hev board search --since` does.
- **IDs and filters.** Path parameters are `{id}`. Repeated query parameters OR
  within a filter; different filters AND; `exclude_*` wins over inclusion.
- **Idempotency.** Creates take `Idempotency-Key`; ingest is idempotent by row
  id; peer messages by client `id`.
- **Versioning.** `/v1` changes only additively, and clients ignore unknown
  fields. A breaking change ships as `/v2` beside `/v1`, with at least one kit
  minor release of overlap. The spec's `info.version` tracks the document, not
  the path.

## Migration and retirement

1. **Spec first (this RFC).** Lint the spec in CI with
   `redocly lint openapi/kit-v1.yaml`.
2. **Serve `/v1` in Pro** behind the existing hosted key. Implement it as thin
   handlers over the existing packages (`internal/trace`, `internal/board/store`,
   `internal/dispatch` RPC actions, `internal/peers` inbox store), so folder
   and API behaviour cannot drift.
3. **Switch clients by storage mode.** `hev traces`, `hev board`, `hev dispatch`,
   `hev steer` and `peers` call `/v1` when the host is in Pro mode, and the
   folders otherwise. Dispatch over SSH stays for OSS clusters.
4. **Retire the fragmented routes** once the clients ship:
   - `pkg/dashboard` hosted `/api/*` → the dashboard UI calls `/v1/traces/*`.
     Local `hev serve` may keep `/api/*` as an internal UI detail; it is not
     documented as an API.
   - Board server `/api/threads…`, `/api/blobs`, `/blob/{sha}` → `/v1/board/*`
     in Pro; `hev board serve` keeps them locally for OSS.
   - `hev collect` `/v1/app/boards/{board}/*` and `/v1/app/traces/*` → `/v1/board/*`
     and `/v1/traces/*`, with app identity carried by `Poster.author_name` and
     trace filters. One release of overlap, then removed.
   - hevd `POST /v1/peers/enqueue` → `POST /v1/peers/{id}/messages` for Pro
     hosts; the hevd listener stays for OSS clusters.
   - Hooks retire nothing: `hev hook` keeps running local handlers and
     writing `events.jsonl` on every host, and in Pro mode also calls
     `POST /v1/hooks/events`.
5. **Docs.** Per memo #534 the old API pages are already dropped. The API
   section returns under the Pro toggle, rendered from `openapi/kit-v1.yaml`.
   The old Field Kit API docs are not in this repo; whichever site still
   hosts them should redirect to the new reference and be deleted when `/v1`
   ships. Field Kit's trace shapes are kept, so nothing written against them
   needs re-mapping beyond the route paths above.

## Open questions

1. **OTLP path.** `POST /v1/traces` (OTLP) and `/v1/traces/*` can coexist, but
   only if the collector and the Pro API stay on different hosts or the Pro
   router sends exactly `POST /v1/traces` to the collector. Which one?
2. **Order ids across hosts.** Run names are unique per host today. `/v1/orders/{id}`
   assumes account-wide uniqueness; the alternative is `{host}:{name}` like peers.
3. **Board delete.** Hard delete (as specced) or a tombstone that keeps the
   number and the reply chain readable to admins?
4. **Kit events through `POST /v1/hooks/events`.** Pro raises `peer-message`
   and `board-post` itself because peers and the board are hosted. Should a
   Pro host also forward the local copies (from OSS-mode tools on the same
   account), or would that double-deliver?
5. **Signing scheme.** `X-Hev-Signature-256` signs `TIMESTAMP.BODY`, unlike
   GitHub's body-only `X-Hub-Signature-256`. That buys replay protection at
   the cost of reusing GitHub verifiers unchanged. Keep it?
6. **Folder paths.** Should the trace archive and peers state move to
   `~/.hev/traces/` and `~/.hev/peers/` to match the memo's mapping, or should
   the docs use the real paths?
