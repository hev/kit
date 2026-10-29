# RFC: FOH traces

Status: draft. Trace-root decision by Adam, 2026-10-29. Builds on kit RFC 0003's "factory join" (`instance`, `plan`, `rfc`, `issue`, `pr`), which was declared but never filled.

Prototype: [`../../prototypes/order-trace/index.html`](../../prototypes/order-trace/index.html). It is the real Luxir docs work from Oct 23–24: 7 asks, 4 legacy orders, 22 worker sessions on Oven and Mini, $81.68, joined by hand.

## Outcome

Adam opens a FOH ask in the kit dashboard and sees the work it started as one trace, in a waterfall:

- the intake and later asks that steer, correct or continue the work
- every order filed for it, with queue, run, validation and acceptance
- worker sessions, turn by turn, including Mini peer-requested checks and runs dispatched outside any order
- Expediter actions and peer messages

From a worker session he can jump to the launching ask and any associated order. From a FOH session he can see the traces it started or continued. The Orders view remains available as a filter within traces.

## Why now

FOH → BOH is now the engineering path (coordination skill, 2026-10-28), but each hop lands in the archive as an unrelated session. Kit captures FOH, workers, validation and shared Expediter shifts without the edges between them.

The Luxir prototype made the missing edge concrete. Seven asks crossed four legacy orders and three runs dispatched outside any order. One trace per order splits the story into five traces and leaves those orderless runs outside the order traces. The by-ask view told the story.

Order UIDs, run IDs, native job IDs and verified FOH caller sessions already identify parts of the work. What is missing is a trace identity carried from the ask through every launch.

## Success measures

- **Coverage:** at least 95% of worker sessions launched in a week carry the trace ID from FOH and their launching run or request span ID, across daemon, interactive dispatch and peer-requested checks. Every FOH intake and follow-up action that applies or dispatches work records its trace and FOH turn.
- **No lost runs:** runs outside any order are visible in the trace, flagged **no order**.
- **One click:** an ask opens its waterfall without manual joins. A worker session links to its launching ask and associated order, when present.
- **Cost:** spans must not push the session list past the target set by `kit-schema-cut-20261029`.
- **Real question test:** the Luxir work can be read as one trace for asks 1–6, with ask 7 opening a new linked trace. For the four kit orders filed on 2026-10-29, Adam can see queue, run and validation time and who touched each, without losing their shared intake.

## Design

### Trace model (OTEL-shaped)

**The FOH ask that starts new work is the trace root: `foh.intake`.** FOH mints a nonzero 128-bit `trace_id` at intake and records it on that FOH turn. It is independent of the order UID. Later asks that steer, correct or continue that work are `foh.ask` child spans in the same trace. Several orders can share a trace; retries and rework stay there too.

Unrelated new work starts a new intake and trace with a span link back to the referring ask. Luxir ask 7, "dispatch a worker to handle 327", is that case. A replacement order alone does not require a new trace if it continues the same work.

| Span | Parent | Source of truth | Start → end |
|---|---|---|---|
| `foh.intake` (root) | none | FOH turn and intake record | starting ask → intake handling complete |
| `foh.ask` | `foh.intake` | later FOH turn | follow-up ask → handling complete |
| `order` | launching `foh.intake` or `foh.ask` | order resource events | apply → Accepted / Cancelled |
| `order.context` | `order` | register / select / freeze events | first register → freeze |
| `run` (one per attempt) | launching action span, or `order` for its queued dispatch | dispatch and daemon run record | claimed → terminal status |
| `validation` | launching action span, or `order` for its queued validation | validation run record | claimed → verdict |
| `peer.request` | launching FOH action | peer send and receipt | request → completion |
| `session` (worker, including Mini checks) | `run`, `validation` or `peer.request` | harness transcript and launch record | first turn → last turn |
| `turn`, `tool` | `session` | kit chunks and blocks | as recorded |
| `acceptance` | `order` | `hev accept` event | instant span |

An order's 64-bit `span_id` is deterministically derived from its UID; the full UID remains the `order_id` attribute. Use a versioned derivation with collision detection. Other span IDs are unique within the trace. IDs identify spans; names such as `foh.ask` describe them.

A span has one parent. A run launched by a follow-up ask uses that action's context and carries `order_id` plus a link to the order span when applicable. Queued launches retain the originating action through the order's parent chain. A run without an order still has a parent and trace; its `order_id` is absent and the UI labels it **no order**. Missing context is reported as untraced, never silently reconstructed from an order UID.

**Shared sessions and messages:**

- A FOH session handles many asks and traces. It is not itself one span; its turns identify the relevant intake or follow-up spans.
- An Expediter shift touches many traces. It keeps span links to the actions and orders it touches. Its order-thread actions are events on the relevant span.
- Peer and order-thread messages are events on the nearest action span, carrying sender, receiver and message key. A peer-requested check also has a request span for its worker parent.
- Skills are events on `session` and `turn` spans, as delivered by `kit-skill-capture-20261029`.

### Propagation: who stamps what

1. **FOH intake and continuation.** FOH records `trace_id`, intake span ID, caller session, host and turn at intake. It carries an active trace per session. A continuation records a new `foh.ask` span under that intake; a new intake starts a new trace. The active context includes the current action span, not only the trace ID.
2. **Apply and dispatch.** `hev apply`, `foh dispatch` and `peers send` stamp that active context, with explicit `--trace` override. An override must resolve a recorded trace and action parent, rather than reuse an unrelated active parent. Apply records the order span and FOH provenance; dispatch records its launching action even without an order. Commands without resolvable context report the gap instead of guessing an intake from the latest prompt.
3. **Launch → worker.** Daemon and interactive launchers persist the incoming context, create the run, validation or request span, then set W3C `TRACEPARENT` with the intake trace ID and that span as parent. They set `HEV_TRACE_ID`, `HEV_RUN` where applicable and `HEV_ORDER` only when associated. The launch record stores the harness session ID next to `native_job`, trace, parent and optional order. Kit joins on that record because transcripts do not record the environment. Queueing and retries preserve the recorded context rather than reading a later active trace.
4. **Expediter and peers.** Messages carry `traceparent`; shared shifts preserve span links across traces. A receiving check inherits the request context even when it has no order.
5. **Kit join.** The indexer reads FOH action records, dispatch/run records, order events and harness sessions. It writes spans and associates worker sessions with them. Chunks and blocks inherit context through `session_id` and are not rewritten. Shared FOH and Expediter sessions use turn associations and links rather than a single session-wide trace stamp.

### Storage

- **New namespace `hev-traces-spans`:** one row per span, shaped like OTLP (`trace_id`, `span_id`, `parent_span_id`, `name`, `kind`, `start`, `end`, `status`, `order_id`, `attributes`, `events`, `links`).
- **Indexed fields:** `trace_id`, `order_id`, `parent_span_id`, `name`, `start`, `session_id`. `order_id` supports the Orders view; attributes, events and links are stored as non-indexed JSON.
- **Worker session rows** gain indexed `trace_id` and `order_id`, with non-indexed `span_id` and `parent_span_id`. Shared sessions retain their per-turn/span associations instead of claiming one trace for the whole session.
- Intake records preserve FOH session and turn provenance. Historical joins without recorded context are marked `inferred`; unknown trace or order associations stay unknown.

### Dashboard

- **Traces / asks** is the entry point: intake asks with phase, follow-up asks, orders, runs, cost and durations.
- A **trace page** opens in the native **By ask** view. It shows orders and orderless runs together, and links to related traces. Ask 7 opens its new linked trace.
- An **Orders** view filters by indexed `order_id` within traces. It offers the full trace for context rather than treating each order as a trace.
- Selecting a worker opens the existing session view with a breadcrumb to trace intake, launching ask, optional order and run/request.

## Milestones

1. **Stamp at FOH:** mint intake IDs, record follow-up spans and active context, add `--trace`, and persist context through apply, dispatch and peers. These are factory/FOH changes outside kit.
2. **Carry through launch:** daemon and interactive launchers record harness session IDs and propagate `TRACEPARENT`/`HEV_*`, including validation, Mini requests and orderless runs. Roll out on Oven, Mini, then win-oven. Preserve Expediter links and message events.
3. **Join:** kit indexes spans by trace and order, with shared-session turn associations. Backfill only where evidence supports the join; label inferred associations and expose gaps. Do not mint historical intake identities from order UIDs.
4. **See and verify:** ship Traces / asks, Orders filtering, waterfall and breadcrumbs. Exercise continuation, several orders per intake, retries, an orderless Mini check, a new linked trace and a mis-attribution correction against the coverage measures.

## Non-goals

- Tracing inside a harness beyond what kit already captures (no model-call spans).
- A live view. Traces show up once kit's indexer has run (minutes).
- Client realms. BCC/win-oven traces stay in their own boundary; this RFC covers the hev realm first.

## Open questions

1. **New or continue.** What rule does FOH use to distinguish unrelated work from a continuation, especially across resumed sessions or several active asks? The intake decision is settled; the operator cue and active-context switching workflow need definition.
2. **Mis-attribution recovery.** How does FOH correct a wrongly inherited trace or order after dispatch? Preserve original provenance and an auditable correction; decide whether the read model reparents spans or links corrected attribution, and how downstream context is repaired.
3. **Export.** Stay OTEL-shaped in Layer first, or also export OTLP? Layer-only first keeps the initial scope small.
4. **Linear.** Should traces also link Linear issues when present (`issue` is already a kit column)?
