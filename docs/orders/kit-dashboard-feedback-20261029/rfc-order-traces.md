# RFC: Order traces

Status: draft for discussion with Adam (2026-10-29). Builds on kit RFC 0003's "factory join" (`instance`, `plan`, `rfc`, `issue`, `pr`), which was declared but never filled.

Prototype: [`docs/prototypes/order-trace/index.html`](https://github.com/hev/kit/blob/work/kit-dashboard-feedback-20261029/docs/prototypes/order-trace/index.html) on kit branch `work/kit-dashboard-feedback-20261029`. It is the real `luxir-docs-deploy-20261023` work from Oct 23–24 (7 asks, 22 worker sessions on Oven and Mini, $81.68), joined by hand, as a waterfall.

## Outcome

Adam opens one order in the kit dashboard and sees its whole life as one trace, in a waterfall:

- the FOH conversation where he asked for it
- the order being filed, pinned and queued
- each run on Oven, Mini or win-oven, and the worker session inside it, turn by turn
- validation and acceptance
- what the Expediter did on it, and the peer messages in between

From any worker session he can jump up to the order and the FOH ask that caused it. From a FOH session he can see every order it started and where each one is.

## Why now

FOH → BOH is now the only way engineering work gets done (coordination skill, 2026-10-28). Each hop lands in the archive as an unrelated session today:

- A FOH session files orders, Oven's daemon starts `codex exec` for each run, the shared Expediter on Oven follows several orders per shift, and validation is another run. Kit captures every one of those sessions, but nothing ties them together.
- Only 1 of 5,912 sessions in the last 30 days is even marked as having subagents.
- The question "why did this order take four hours and three runs" means reading four transcripts by hand.
- The order system already has the identities a trace needs:
  - an order UID, which is 32 hex characters, exactly the size of an OTEL trace ID
  - a run ID (`<uid>:<attempt>`)
  - a thread ID
  - the daemon's native job ID
  - FOH's verified caller session

Nobody stamps those identities onto the sessions. That's the gap.

## Success measures

- **Coverage:** at least 95% of worker sessions started by the daemon in a week carry the trace ID of their order and the span ID of their run. The same goes for 100% of FOH sessions that ran `hev apply` for an order.
- **One click:** from an order, the dashboard renders the full waterfall with no manual joins. From a worker session, one click reaches the order and the FOH ask.
- **Cost:** adding spans does not slow the dashboard. It must not push the session list past the target set by the schema cut (`kit-schema-cut-20261029`).
- **Real question test:** for the four kit orders filed on 2026-10-29, Adam can answer "how long did each wait in the queue, run and sit in validation, and who touched it" from the dashboard alone.

## Design

### Trace model (OTEL-shaped)

**One trace per order. `trace_id` = order UID.** A retry is a new run inside the same trace. Rework stays in the same trace. A replacement order is a new trace, with a span link to the one it replaced.

| Span | Parent | Source of truth | Start → end |
|---|---|---|---|
| `foh.intake` (root) | none | FOH session transcript | Adam's prompt that led to the order → `hev apply` |
| `order` | `foh.intake` | order resource events | apply → Accepted / Cancelled |
| `order.context` | `order` | register / select / freeze events | first register → freeze |
| `run` (one per attempt) | `order` | daemon run record | claimed → terminal status |
| `session` (worker) | `run` | harness transcript (already in kit) | first turn → last turn |
| `turn`, `tool` | `session` | kit chunks and blocks (already have start, end, ms) | as recorded |
| `validation` | `order` | validation run | claimed → verdict |
| `acceptance` | `order` | `hev accept` event | instant span |

**Things that aren't children:**
- *The Expediter.* One shift touches many orders, so its session gets **span links** to each order it acted on. Its order-thread actions show up as **events** on that order's span.
- *Peer messages and order-thread messages:* events on the nearest span (sender, receiver, key), never their own trace.
- *FOH sessions:* they handle many asks. The session isn't a span. Each intake inside it is.

**Skills:** they are events on `session` and `turn` spans, as delivered by `kit-skill-capture-20261029`. They appear in the waterfall, so "what context did this run have" is visible on the trace.

### Propagation: who stamps what

1. **FOH → order.** `hev apply` already verifies the caller session. It records `foh_session`, `foh_turn` (the turn ID that ran apply) and `foh_host` on the order as annotations. The intake span starts at the most recent Adam prompt before that turn. If FOH can't be identified, the order still gets a trace, just with no `foh.intake` span. It is never guessed.
2. **Order → run → worker.** The daemon already knows the order UID and run ID when it launches the worker. It sets `TRACEPARENT` (W3C: trace = order UID, parent = run span ID) plus `HEV_ORDER`, `HEV_RUN` and `HEV_TRACE_ID` in the worker's environment. It also records the harness's own session ID in the run record next to `native_job`. That second part matters most: transcripts don't record the environment, so kit joins on the run record's harness session ID.
3. **Expediter and peers.** `peers send` and order-thread `hev send` carry `traceparent` when the sender has one, and the Expediter's launcher stamps the order UIDs it was handed.
4. **Kit join.** Kit's indexer reads run records (the daemon's journal or `hev get runs`) and order events. It writes span rows and stamps `trace_id`, `span_id` and `parent_span_id` onto the existing session rows. Chunks and blocks inherit them through `session_id` and are not rewritten.

### Storage

- **New namespace `hev-traces-spans`:** one row per span, shaped like OTLP (`trace_id`, `span_id`, `parent_span_id`, `name`, `kind`, `start`, `end`, `status`, `attributes` as non-indexed JSON, `events` as non-indexed JSON, `links`).
- **Indexed fields, staying with the schema cut:** `trace_id`, `parent_span_id`, `name`, `start`, `session_id`. Everything else is stored but not indexed.
- **Session rows** gain an indexed `trace_id` and a non-indexed `span_id`/`parent_span_id`.

### Dashboard

- An **Orders** view: a list of traces with phase, wait/run/validate durations and runs.
- An **order page** with the waterfall. Selecting a `session` span opens the existing trace view.
- The session trace view gets a breadcrumb: FOH ask → order → run → this session.

## Milestones

1. **Stamp:** the daemon records the harness session ID and sets `TRACEPARENT`/`HEV_*` on worker launch, and `hev apply` records the FOH caller. These are factory/daemon changes in `hev/factory`. They need a node rollout on Oven, then Mini, then win-oven.
2. **Join:** kit builds `hev-traces-spans` from order and run records plus session rows, and stamps `trace_id` on sessions. Includes a backfill for orders since the 0.4 context release (orders whose runs never recorded a harness session ID are joined by native job and timestamps, marked `inferred`).
3. **See:** the Orders view, the waterfall and the breadcrumb.
4. **Expediter and peers:** span links and message events.

## Non-goals

- Tracing inside a harness beyond what kit already captures (no model-call spans).
- A live view. Traces show up once kit's indexer has run (minutes), which is fine for review.
- Client realms. BCC/win-oven traces stay in their own boundary; this RFC covers the hev realm first.

## Open questions

1. **Export.** Should spans also go out over real OTLP to a collector (Jaeger, Honeycomb), or stay OTEL-shaped inside Layer and the kit dashboard? I'd recommend Layer-only first, with OTLP export as a thin later add-on, because the IDs and shapes already match.
2. **What counts as the intake.** The span runs from the last Adam prompt before `hev apply`. When one prompt produces four orders (as on 2026-10-29), all four share that prompt. Do we want a shared intake span with four children, or four intake spans? I'd recommend shared: one intake, several orders.
3. **Interactive peers.** `foh spawn`/`peers spawn` workers outside the daemon: should they be traced in this release, through the same `TRACEPARENT` from `foh spawn`, or left for later?
4. **Linear.** Should a trace also link the Linear issue when there is one (`issue` is already a kit column)?
