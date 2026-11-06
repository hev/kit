# Deterministic watches

Status: initial implementation, with explicit future composite-source example.

A watch binds a named source to one or more deterministic when clauses. Each clause
has an action identifier, execution mode, and its own instruction template. Effects
are dispatch, comment, or notify. Source adapters fetch normalized records; source
code owns authentication and authorization. Clauses cannot weaken the Linear active,
non-guest human-authorizer check or email's sender allowlist and DMARC requirement.
There is no model call deciding whether or how to admit work.

Legacy intake TOML maps to this model during load. Status rules become
`status-becomes` clauses, mention rules become `mentioned`, and the email rule
becomes `email-arrives`. Existing lease names, markers, batching, limits, lock,
restart guard, reconciliation, backoff, and per-ticket dispatch isolation remain.
One source adapter interface supplies typed snapshots for Linear issues, mentions,
and email. Different status clauses can dispatch different instruction templates.
The initial implementation allows one clause per status, retaining the original
unambiguous rule selection. Comments and notifications are unbatched effects.

The source guard runs before any effect. Before a live comment or notification,
the ledger records a pending effect and charges the daily cap. A lost or rejected
response becomes needs-reconcile. No automatic replay is permitted until a human
confirms whether the remote effect happened. Notifications receive a stable
idempotency key and never follow redirects. Persisted completed effects suppress
repeat delivery. Dry runs produce decisions without external effects.

## Implemented example: delivery instructions

A Linear delivery watch has a Ready clause dispatching `orders/build.md`, a Review
clause dispatching `orders/review.md`, and a Shipped clause commenting from
`messages/shipped.md`. The team, assignee, and allowed human team are shared source
settings. `docs/intake.md` gives the executable configuration shape.

## Future example: health and shipped status

The desired composition is a watch over a tracker snapshot and an HTTP health
snapshot. This illustrative syntax is a proposal, not accepted configuration:

```toml
[[watch]]
name = "delivery-health"
[[watch.sources]]
name = "ticket"
kind = "linear"
statuses = ["Shipped"]
[[watch.sources]]
name = "service"
kind = "http-health"
url = "https://service.example.invalid/health"

[[watch.when]]
all = ["ticket.status == Shipped", "service.status == 200"]
action = "confirm-health"
effect = "comment"
target = "ticket"
template = "messages/healthy.md"
```

The comment template could say `The service health check passed for {{identifier}}.`
A future implementation must define correlation, freshness, and stable identities:
a health snapshot belongs to a configured service, a ticket maps explicitly to that
service, and a stale or failed probe never satisfies the clause. The source guard
on the target tracker ticket still applies before commenting. The composite lease
must include the ticket's Shipped entry and the relevant health transition so a
repeat poll cannot spam comments. Snapshot acquisition must not perform effects.

Do not add HTTP or shell-command sources in this PR. A later shell-command source
would require bounded runtime/output and a reviewed execution boundary; it would
supply data rather than select actions. The present loader rejects unsupported
sources and conditions. This avoids implying that a partial composite check ran.

## Command ownership and rollout

`hev watch` owns this workflow; `hev intake` is a Cobra alias sharing the exact
handler. `watch run|once|check|render` accepts watch or legacy intake TOML; the
`linear` spelling remains supported. `watch install|start|stop|status` manages the
service; `watch linear status` reads the poller's local state. Drop the former
resource-backend watch verb and use get/describe for observation. Keep existing
intake service identifiers so installation updates the same job names.

Rollout is a reviewed service swap, then removal of the old plugin and manually
scheduled task. No host migration, release, or merge is part of this change.
