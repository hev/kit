# Orchestrators

`hev orchestrate` runs only after an explicit `apply` or `start`. It installs
no daemon, service or background poller. `ls` and `apply --dry-run` create no
state. Each round starts a fresh harness with `HEV_ORCHESTRATOR` set; dispatch
admission checks the applied scope and authority before routing or mutating.
There is no merge authority.

Save the spec from [RFC 0008](rfcs/0008-orchestrators.md) between Markdown
frontmatter delimiters (`---`), followed by a standing brief. The JSON Schema
in `schemas/orchestrator.schema.json` describes its frontmatter. The runtime
validator also checks selectors, dates, durations and the applied nesting graph.
Durations accept Go duration syntax or days, for example `20m` and `7d`.
Loops require `every` and a positive `max_lifetime`.

```sh
hev orchestrate apply -f release.md --dry-run
hev orchestrate apply -f release.md
hev orchestrate start --name follow-up --goal 'Follow my open runs' --runs mine --reply owner
hev orchestrate ls --json
hev orchestrate show release-train
hev orchestrate logs release-train --round 1
hev orchestrate pause release-train
hev orchestrate resume release-train
hev orchestrate wake release-train
hev orchestrate handoff release-train --to next-owner
hev orchestrate stop release-train
hev orchestrate render intake --event event.json
hev orchestrate reconcile intake --event TASK-1 --not-admitted
```

Applied specs live in `~/.config/hev/orchestrators/`. Runtime state lives in
`~/dispatch/orch-NAME/`: `spec.md`, `state.json`, `checkpoint.md`, `inbox.md`,
`budget.json`, and `rounds/NNN/{packet.json,checkpoint.md,trace}`. Use `hev orchestrate say NAME MSG` (or `hev dispatch say o-NAME MSG`) to append
a message and gather early. A wake never overlaps a running
round. `pause` prevents further admissions and rounds; `stop` ends the loop.
Reapply a stopped spec to start a new lifetime while preserving its checkpoint
and round numbering. A failed or empty checkpoint never replaces the last
accepted checkpoint. Idle rounds compare evidence and inbox with the last
accepted checkpoint's digest and avoid calling the model if unchanged.

Checkpoint instructions require goal status, open threads by run name, named
waits with release conditions, next actions and notes for `reply`. For
`until: goal`, a completed goal also writes `goal-met.json` with `{"met":true}`
in the round directory. `rotate_after` asks for a short replacement summary;
previous checkpoints are archived under `lineages/`. `handoff --to` writes a
final checkpoint addressed to the new owner, notifies both owners, and ends
the supervisor.

Scope is a list of names, `mine` (matching `reply`), or `issue=GLOB`, optionally
intersected with repository labels. Authority defaults to `say` and `notify`.
A `say` that would restart an ended run additionally requires `rework` authority.
Run budget limits of zero mean unlimited. Positive `concurrent_runs` and
`runs_per_day` limits count scoped live runs and admitted launch attempts;
failed launch attempts consume the daily allowance. Daily counters reset at
UTC midnight. Target admission and budget reservations share dispatch's lock. The owner
also reserves across the selected target and known scoped hosts under a separate
lock held through launch acknowledgement; unavailable relevant hosts fail admission.

Nested orchestrators must be explicitly named in scope (use `orch-NAME` to
avoid ambiguity with ordinary runs). Apply the child first; the parent must
set `allow_nested: true`. Admission rejects cycles and chains deeper than two
orchestrators, including changes that would invalidate existing parents.

Unix uses a local tmux supervisor in `o-NAME`; Windows uses a detached supervisor and ordinary dispatch routing for its
managed runs. The `host` field is a placement label; apply/start must run on
the intended supervisor host. Remote targets register the same spec with
`apply -f FILE --scope-only-owner OWNER` (a configured remote owner host). This
starts no supervisor and checks lifecycle with the owner on each admission; the owner enforces run budgets across its configured inventory and targets
additionally enforce their local caps.
`usd_per_day` is retained as informational configuration; dollar enforcement
requires later cost accounting. Inventory includes run status, reports, verdicts
and current PR/CI data; collection failures prevent a model round.

Source plugins, deterministic `agent: off` rules, and the expedite preset are
implemented. Source intake records a receipt per event. Each event ID must be
a transition ID: stable when the same transition is replayed, and new when an
item returns to a status, or the re-entry is skipped as a replay. A bad event is
rejected and retried after 1, 2, 4 ... 64 polls without blocking the rest of the batch. Caps
and `pause_file` defer work rather than failing the round. An uncertain launch
is promoted when its run appears, or waits for
`hev orchestrate reconcile NAME --event ID --admitted|--not-admitted`.
`hev orchestrate render NAME --event FILE|-` previews a rule's order without
writing state, and `show`/`ls --json` list intake health signals. See [Phase 2 contracts](rfcs/0008-orchestrators.md#phase-2-contracts)
for the stable JSON inventory, source wire protocol, templates, receipt recovery
and scheduling semantics. No real tracker plugin is included. Picker and shell
integration, the separate peers CLI, and handoff to another orchestrator remain
outside this phase.

`source.env: [NAME, ...]` lists environment variables, such as a tracker token,
that only the source plugin receives. Every orchestrator spec on a host,
including scope-only registrations, contributes its names. Dispatch removes
them from every other child it starts on that host: the round harness, the
worker supervisor, the worker harness, notification senders and their
launchers, and helper commands such as Git, `gh`, SSH and tmux, on Unix and
Windows. A plugin sees only its own spec's names. Values still come from the
orchestrator supervisor's environment; specs store names only. The supervisor
is the one control process that keeps them: at startup it moves them out of
its own process environment and passes each only to the plugin that declares
it, so no other child can inherit them. Names must be
valid environment identifiers, and `PATH`, `HOME` and other names dispatch
needs to start children are refused.

Rounds default to a `round_timeout` of `30m`; timeout fails the round and kills
the harness tree. Accepted inbox messages move to `rounds/NNN/inbox.md`.
Nested specs must be subsets and share their parent's run budget. Terminal
wake/resume/handoff calls fail until reapply. See RFC 0008's trust boundary:
these admission checks require trusted harnesses and do not replace sandboxing.

## Owner notices

With `reply` set, code sends one-line lifecycle notices naming the orchestrator
and host: started/applied, paused, resumed, stopped, goal met (complete), handoff,
budget exhaustion, and the second consecutive round failure or timeout. These
notices do not require `notify`. Handoff sends to both the old owner and `--to`.
A successful round resets the failure streak. Lifetime/deadline expiry reports
budget exhaustion rather than claiming the goal was met. Admission refusals for
`concurrent_runs` or `runs_per_day` report budget exhaustion at most once per
cap per UTC day. Notice text is limited to 200 characters, with a pointer to
`hev orchestrate show NAME` for the full record.

A source run cap is a deferral, not a failure: the round stays `waiting`,
creates no round directory and sends no failure notice. One informational
notice goes out per cap episode, from the first capped round until a round
runs without a cap deferral.

Intake health signals (`attempt-stale`, `rejected`, `needs-reconcile`) become
owner notices through `dispatch.IntakeSignalHook`. The hook only queues the
notice under the signal's stable ID and acknowledges the signal once that
receipt is on disk. A failed write leaves the signal pending, and a
redelivered signal reuses the queued receipt, so each signal notifies once.

When the owner needs to hear a decision, blocker, ready-to-merge finding (head
SHA and evidence), goal completion or rework sent, a model round writes a short
`reply.md` beside its checkpoint. Code delivers it only after accepting the
checkpoint, only with `notify` authority, and at most once per round. The
checkpoint's reply notes remain the record. No file means no model message;
failed and skipped rounds create none. Messages over 2,000 characters are
truncated with a pointer to `hev orchestrate show NAME`.

Delivery uses native `hev peers send` with an explicit `HOST/o-NAME` sender,
a stable message ID and `--inbox-only`. `HOST` is the spec's `host`, or the
peers host label dispatch derives for its own notices; apply rejects a label
peers would refuse. Owners must register a peers inbox
(`hev peers inbox on SESSION`); terminal pasting cannot provide retry deduplication.
A delivered status means accepted by the inbox, not read or acknowledged.
No inherited harness sender or external peers backend is used.

Receipts live outside `rounds/`, in `notices/KIND/HASH.json` (`KIND` is
`reply`, `lifecycle` or `intake`). The hash covers the event key and
recipient, and the outbound ID is `HOST/o-NAME:KIND:HASH`, so the ID, payload
and recipient stay fixed across rounds, retries and redelivery. Lifecycle keys
are scoped to the lineage, never the round, so a stop that races a round sends
once. Each receipt holds `id`, `to`, `status`, `error`, `message`, `created`,
`kind` and `attempts`.

Code writes and fsyncs receipts under the orchestrator's notices lock. Rounds,
controls and admission only queue. Delivery happens after the dispatch root
lock and the orchestrator lock are released: the supervisor delivers on each
wake and after each round, and a control delivers itself when no supervisor is
live. Apply, say and admission never wait on a send. Each send has a 20 second
deadline. Per recipient, delivery goes oldest first and stops at the first
failure. The failed receipt stays `queued`, counts the attempt and backs off
(1 minute, doubling to 1 hour), so an unreachable owner costs one bounded
attempt per wake. Delivered receipts are never sent again; recipient inboxes
deduplicate stable IDs if delivery succeeded before a receipt could be
recorded. An inbox that refuses a reused ID for a different recipient (after
an owner restart) makes the receipt terminal `failed`. Reapply marks queued
receipts `stale`, so nothing queued before a stop is sent to the new lineage.
Delivery errors never fail a round. A notice on a scope-only target is
skipped; the owner host speaks for the orchestrator.

### Pull route

A host that cannot reach the owner, or an owner without tmux (for example on
Windows), sets `notify_route: pull`. Receipts are written `queued` as usual,
and no transport is ever executed. The owner pulls them:

```bash
hev orchestrate notices pull --host HOST            # list pending notices as JSON
hev orchestrate notices pull --host HOST --deliver  # deliver locally, then ack
```

`--deliver` puts each notice into the local peers inbox under its stable ID,
then acknowledges it on `HOST`. If the ack is lost, the next pull delivers the
same IDs again and the inbox deduplicates them. Both steps are RPC actions
through `ssh HOST hev dispatch rpc`: `orchestrator-notices` (pending receipts,
oldest first; `name` limits it to one orchestrator) and
`orchestrator-notice-ack` (`ids`). An acknowledged receipt becomes `delivered`.

### Seed checkpoint

`hev orchestrate apply -f SPEC --seed-checkpoint FILE` records an existing
checkpoint as round `000` before round `001` runs. The file must be UTF-8 and
include `Goal status`, `Open threads` and `Next action` sections (Markdown
headings or labels). It is written to `rounds/000/checkpoint.md` with
`rounds/000/provenance.json` (`source` path, `sha256`, `time`) and becomes the
checkpoint round `001` reads. Seeding the same content again is a no-op; a
different seed, or any seed after round `001`, is refused.

### Remote apply trust boundary

The `orchestrator-apply` RPC action (used by `hev expedite` to apply on a
remote host) accepts a spec from any caller that can run `hev dispatch rpc` on
that host, which in practice means anyone with ssh access as the dispatch
user. That caller can already run any `hev` command, so the action grants no
new authority, but remote apply is not restricted to the owner. Limit ssh
access to the host accordingly.

`show` includes `last-notice.json`; `ls --json` includes `last_notice` (`null`
before any notice) and `consecutive_failures`. The last notice contains its
delivery status and error; retrying an older notice does not hide a newer one.

The stable `ls --json` inventory includes the applied spec’s `reply` owner peers
address (empty string when unset) and optional `host` placement label (absent
means local owner). Consumers can match `reply` to find a session’s owned
orchestrators. Reapply updates both fields. See the RFC’s stable inventory
contract for the complete field list.
