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
