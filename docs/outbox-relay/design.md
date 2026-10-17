# Transactional outbox projection

`pkg/outboxrelay` is an opt-in library, with no daemon hook, credential lookup,
provisioning or migration. The caller supplies a database handle, destination,
consumer identity, live UUID lease holder/fence and a Layer implementation.
All writers of one destination must use the lease `outbox-relay/<destination>`.
The caller acquires/renews/releases it using the kitchen v1 functions.

Each event is an immutable history document, rather than an overwrite of an
order's current state. `ProjectEvent(order)` reads the next dense per-order
sequence and atomically enqueues it with a fenced CAS cursor update. The
idempotency key includes format version, order identity and sequence. A caller
iterates orders from the database and drains each stream; it must keep polling
open orders. Event timestamp, source, kind, digest and original payload survive.

`Relay.Step` claims one pending row before external I/O. Document IDs are SHA256
of the JSON tuple (destination, idempotency_key); payloads and order IDs are
retained. One document per write avoids assuming batch atomicity. Partial batch
failure is therefore a series of independent steps. Successful completion and a
per-row completion cursor are committed together. Pending rows are NEVER filtered
by a global outbox cursor: bigserial allocation order is not commit order.

Every SQL transaction checks API v1 and the live destination fence. No external
request runs inside a database transaction. Fences protect database mutations,
not a request already in flight. Immutable IDs/content make delayed duplicate
writes harmless. A takeover cannot reclaim sending rows for another write.

After any transport error, or a crash between claim and completion, the original
claim remains sending. Once expired, the relay inspects by exact document ID.
Only a positive observation with matching full content permits completion.
Absent/lagged reads remain uncertain; different content is a conflict. This
version deliberately cannot turn an uncertain row back into pending: the v1
schema has no evidence-bearing outbox retry API. Operators must investigate
uncertainty; they must not reset rows or blindly replay sends. Unresolved rows
remain visible in Postgres. The relay does not sweep unrelated destinations.

To rebuild, use a new empty Layer namespace, a new destination and consumer,
then replay all order event streams from zero. To include non-event outbox
history, enqueue its retained payload/order/key under the new destination using
`enqueue_outbox` (SELECT retained rows, never a global high-water cursor).
Original rows, attempts, cursors and uncertain receipts remain untouched. Route
reads to the new namespace only under a separately authorized activation plan.

Validation uses synthetic fixtures and an ephemeral local PostgreSQL only.
No hosted database, credential resolution or live Layer write is required.

The concrete Layer adapter uses a dedicated namespace with `upsert_rows`, one
row per call, and exact-ID filtered queries. It retains a JSON string payload,
key and order attributes, plus a full-text searchable `text` field. It requires
one acknowledged upsert; ambiguous or malformed responses are reconciled. It
assumes no server transaction or conditional-write capability. The caller must
bind each destination to one dedicated namespace and preserve that binding.

Run unit/wire tests with `go test -race ./pkg/outboxrelay`. Local SQL acceptance
also requires `OUTBOX_TEST_PG_BIN` (server binaries), `OUTBOX_TEST_SCHEMA` (the
validated kitchen v1 SQL file), and optionally `OUTBOX_TEST_PG_SHARE` for unpacked
server distributions. The harness ignores database DSNs and creates an isolated
Unix socket server. Without these variables SQL acceptance explicitly skips.

Event outbox intents use compact references and hydrate immutable source rows
before delivery. This preserves events up to the source payload bound without
exceeding the outbox bound. Event timestamps use a fixed UTC representation.
The generated event idempotency-key tuple and matching reference envelope are
reserved together; an ordinary payload merely containing a `projection` field
continues to be delivered as ordinary JSON. Reconciliation scheduling records
rotate unresolved observations so one absent row cannot block later matches.
