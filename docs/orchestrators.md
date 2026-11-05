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
budget exhaustion rather than claiming the goal was met. Run cap refusals also
report budget exhaustion; no cost accounting is added.

When the owner needs to hear a decision, blocker, ready-to-merge finding (head
SHA and evidence), goal completion or rework sent, a model round writes a short
`reply.md` beside its checkpoint. Code delivers it only after accepting the
checkpoint, only with `notify` authority, and at most once per round. The
checkpoint's reply notes remain the record. No file means no model message;
failed and skipped rounds create none. Messages over 2,000 characters are
truncated with a pointer to `hev orchestrate show NAME`.

Delivery uses native `hev peers send` with an explicit `HOST/o-NAME` sender,
a stable message ID and `--inbox-only`. Owners must register a peers inbox
(`hev peers inbox on SESSION`); terminal pasting cannot provide retry deduplication.
A delivered status means accepted by the inbox, not read or acknowledged.
No inherited harness sender or external peers backend is used.

The reply receipt is `rounds/NNN/notice.json`. Lifecycle receipts are separate
`rounds/NNN/notices/EVENT/notice.json` files (round `000` is before the first
round). Each holds `id`, `to`, `status`, `error`, `message` and `created`.
Receipts are written before attempting delivery. Unreachable recipients and
uncertain outcomes stay `queued` with an error and retry on subsequent rounds
or supervisor wakes, including idle gathers. Delivered receipts are never sent
again; recipient inboxes deduplicate stable IDs if delivery succeeded before a
receipt could be recorded. Payloads and recipients stay fixed across retries.
Delivery errors never fail a round. There is no background retry service: a
terminal orchestrator's queued notices wait until a later explicit supervisor
wake or reapply. This does not add restart/reboot recovery.

`show` includes `last-notice.json`; `ls --json` includes `last_notice` (`null`
before any notice) and `consecutive_failures`. The last notice contains its
delivery status and error; retrying an older notice does not hide a newer one.

The stable `ls --json` inventory includes the applied spec’s `reply` owner peers
address (empty string when unset) and optional `host` placement label (absent
means local owner). Consumers can match `reply` to find a session’s owned
orchestrators. Reapply updates both fields. See the RFC’s stable inventory
contract for the complete field list.
