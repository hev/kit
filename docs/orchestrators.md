# Orchestrators

`hev orchestrate` runs only after an explicit `apply` or `start`. It installs
no daemon, service or background poller. `ls` and `apply --dry-run` create no
state. Each round starts a fresh harness with `HEV_ORCHESTRATOR` set; dispatch
admission checks the applied scope and authority before routing or mutating.
`merge` authority is opt-in (for example via `hev orchestrate follow --authority merge-on-green`) and authorizes merging an exact reviewed head only when all checks are green.

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
hev orchestrate adopt release-train --to macbook/foh-hev-3
hev orchestrate retire release-train --to next-owner --yes
hev orchestrate stop release-train
hev orchestrate ls --orphaned
hev orchestrate show release-train --host mini
hev orchestrate render intake --event event.json
hev orchestrate reconcile intake --event TASK-1 --not-admitted
```

Applied specs live in `~/.config/hev/orchestrators/`. Runtime state lives in
`~/dispatch/orch-NAME/`: `spec.md`, `state.json`, `checkpoint.md`, `inbox.md`,
`budget.json`, and `rounds/NNN/{packet.json,checkpoint.md,trace}`. An applied
spec whose state directory is missing or unreadable is listed as `broken`
(with the spec path to apply again or remove), and the rest of the host's
inventory is unaffected. Use `hev orchestrate say NAME MSG` (or `hev dispatch say o-NAME MSG`) to append
a message and gather early. `say` is a steering `note`: the next round reads it,
and it is recorded as a receipt (`orchestrate say` prints its `st-...` ID) that
is injected at the round's first prompt.
`hev steer o-NAME MSG --level urgent` reaches a round that is running at its
next tool call, and `--level halt` ends that round's turn; the orchestrator keeps
its schedule (see [steering](steering.md)). A round halt that has not ended its
turn within `--halt-timeout` (default `10m`) becomes a hard stop of the round,
like a dispatch run's; a round launched without kit's hooks gets SIGINT instead. Rounds run with kit's hooks wired, so
`orchestrate show` lists steering receipts. A wake never overlaps a running
round. `pause` prevents further admissions and rounds; `stop` ends the loop.
Reapply a stopped spec to start a new lifetime while preserving its checkpoint
and round numbering. A failed or empty checkpoint never replaces the last
accepted checkpoint. Idle rounds compare evidence and inbox with the last
accepted checkpoint's digest and avoid calling the model if unchanged.

Checkpoint instructions require goal status, open threads by run name, named
waits with release conditions, next actions and notes for `reply`. For
`until: goal`, a completed goal also writes `goal-met.json` with `{"met":true}`
in the round directory. `rotate_after` asks for a short replacement summary;
previous checkpoints are archived under `lineages/`.

## Owners: adopt, retire and orphans

`reply: HOST/SESSION` names the owner, one front-of-house session. When that
session is cleared or closed, every notice fails (`no pane ... matches`) and
the orchestrator keeps working for nobody.

- `hev orchestrate adopt NAME [--to HOST/SESSION] [--host HOST]` re-points a
  live orchestrator's owner without stopping it: rounds, checkpoint, runs and
  state are untouched. It rewrites `reply` in the stored spec, and a running
  supervisor picks it up between rounds without a restart (an `adopted.json`
  marker wakes it; no round starts). Notices still queued or failed for the
  old owner are queued again for the new one (a `notify_route: pull` spec
  keeps them pullable), and an `adopted` lifecycle notice names both owners.
  `--to` defaults to your own peers session. A terminal orchestrator
  (complete, handed-off or stopped) is refused: set `reply` and `apply` again.
  With `runs: mine` the scope follows the new owner's runs; adopt the old
  owner's runs too (`hev dispatch adopt`).
- `hev orchestrate retire NAME --to SESSION` **ends** the orchestrator: it
  writes a final checkpoint addressed to SESSION, notifies both owners, and
  the state becomes `handed-off` (terminal; the on-disk name is unchanged). It
  asks for confirmation, and needs `--yes` when stdin is not a terminal.
  `handoff` is its deprecated name: it refuses without `--yes`, because
  "handoff" reads like adopt and has ended orchestrators by mistake.
- `hev orchestrate ls` has an OWNER column: the reply and whether that session
  is `live` in peers, `gone` (its host answered without it) or `asleep` (its
  host did not answer, as a closed laptop does). `--json` carries `owner` and
  `owner_status`.
- `hev orchestrate ls --orphaned` lists non-terminal orchestrators on every
  reachable dispatch host whose owner is gone, each with the exact `adopt`
  command. Asleep owners are not orphans. Nothing runs at session start; ask
  when you want to know.

`show`, `logs`, `wake`, `pause`, `resume`, `stop`, `adopt` and `retire` accept
`--host HOST` to act on another machine's orchestrator over dispatch RPC.
Without `--host` they look locally, then on reachable dispatch hosts, and
refuse a name found on more than one host.

Scope is a list of names, `mine` (matching `reply`), `issue=GLOB`, or
`swarm=NAME`, optionally intersected with repository labels. `swarm=NAME` must
name the orchestrator itself: it matches runs whose metadata records
`swarm: NAME`, which target admission writes only from that orchestrator's
identity on `start`, never from order frontmatter. `hev orchestrate swarm up` compiles it
([swarms](swarms.md)). Authority defaults to `say` and `notify`; the other
verbs are `rework`, `retry`, `validate`, `dispatch` and `stop` (RFC 0013), which
stops a scoped run the way `hev dispatch stop` does.
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

Unix uses a local tmux supervisor in `o-NAME`; Windows starts the supervisor
through its own scheduled task (below) and uses ordinary dispatch routing for
its managed runs. The `host` field is a placement label; apply/start must run on
the intended supervisor host. Remote targets register the same spec with
`apply -f FILE --scope-only-owner OWNER` (a configured remote owner host). This
starts no supervisor and checks lifecycle with the owner on each admission; the owner enforces run budgets across its configured inventory and targets
additionally enforce their local caps.
`usd_per_day` is retained as informational configuration; dollar enforcement
requires later cost accounting. Inventory includes run status, reports, verdicts
and current PR/CI data; collection failures prevent a model round.

### Windows supervisors

On Windows, `apply`, `start`, `resume`, `wake` and `say` start the
supervisor through a Task Scheduler task, `hev\orch-NAME`, that hev registers
for that one supervisor: on demand (no trigger), as the applying user without
a stored password (S4U, the same principal as hevd's `hev\hevd`), at least
privilege, with no time limit. A run's supervisor starts the same way as
`hev\run-NAME`, including a rework or dispatch made from inside an
orchestrator round. Neither belongs to the console or job object of the
process that launched it, so closing the SSH session, or the end of the round
that dispatched it, leaves it running.

The task does not inherit the launching environment, so hev writes it to
`supervisor-launch.json` (readable only by you, SYSTEM and Administrators) in
the orchestrator's or run's state directory; the supervisor reads and deletes
it before it does anything else, and writes its output to `supervisor.log`
there. A supervisor started by its task (`supervise --scheduled`) refuses to
run without that file. When the supervisor exits it writes its exit code and
error to `supervisor-exit.json` and removes its task; `stop`, `retire`,
`hev dispatch stop` and `clean`, and a run whose launch has lapsed remove the
task and both files when no supervisor is left to.

A launch waits up to 30 seconds for the supervisor's `supervisor.json`. When
the supervisor exits first, the command fails with
`supervisor launch unconfirmed`, the task name, the supervisor's exit code and
error, and the log path (an exit with code 0 counts as started). When the task
ends before its supervisor starts, the message gives the task's exit code and
the last log line instead. When 30 seconds pass with neither, hev withdraws the
launch: it deletes the environment file and the task, so a supervisor the task
starts late refuses to run, and the command fails saying what the task was
doing. If the supervisor had already read its launch by then, it is left to
finish and the message says it may still confirm. None of these files travel
with an amend transfer.

Hand-made scheduled tasks that run `hev orchestrate supervise NAME` (for
example `hev-orch-NAME`) are no longer needed: stop the orchestrator, delete
them with `schtasks /Delete /TN hev-orch-NAME /F`, and apply or resume again.

`apply` refuses while the supervisor or a round's harness may still be
running, on every platform: `stop orchestrator before applying changes`. The
refusal leaves `state.json`, the spec and the round in progress untouched. A
recorded process Windows will not let you query counts as running when the
process snapshot still shows `hev.exe` under its PID; if the PID now belongs to
another program, the record is stale and counts as exited. In that unqueryable
case `resume` and `wake` fail and name the record rather than report a start;
stop the orchestrator, or delete the named `supervisor.json` if that process is
not a hev supervisor.

Source plugins, deterministic `agent: off` rules, and the follow-through preset are
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
wake/resume/retire/adopt calls fail until reapply. See RFC 0008's trust boundary:
these admission checks require trusted harnesses and do not replace sandboxing.

## Owner notices

With `reply` set, code sends one-line lifecycle notices naming the orchestrator
and host: started/applied, paused, resumed, stopped, goal met (complete), handoff,
budget exhaustion, adopted, and the second consecutive round failure or timeout. These
notices do not require `notify`. Retire (handoff) sends to both the old owner and `--to`;
adopt sends its notice to the new owner.
A successful round resets the failure streak. Lifetime/deadline expiry reports
budget exhaustion rather than claiming the goal was met. Admission refusals for
`concurrent_runs` or `runs_per_day` report budget exhaustion at most once per
cap per UTC day. Notice text is limited to 200 characters, with a pointer to
`hev orchestrate show NAME` for the full record.

A round whose `reply.md` first line is `[needs you] needs-decision KEY:
QUESTION` (an open decision) also raises a `needs-you`
[attention notice](notify.md), with or without `notify`, once per subject: an
orchestrator repeating the same question every round raises it once. Any
other `[needs you]` reply (a stalled or failed run, a missing tool) and the
stalled notice are `done` records, not `needs-you`. Goal met raises `ready`
and stop raises `done`. The notice runs
`hev-hook-NAME` handlers on the orchestrator's host and posts to Slack when
that host has `[notify.slack]`. Posting does not depend on the route, so a
pull-route host with Slack config posts too. The owner receipt carries the
notice, so a pulling host with Slack config posts it as well. A failed post
never fails a round. `hev orchestrate show` lists the deliveries in
`attention.json`. Its notices are for whoever applied the orchestrator
(`requested_by` in `state.json`), and so are the runs it dispatches
([who a notice is for](notify.md#who-a-notice-is-for)).

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

The `orchestrator-apply` RPC action (used by `hev orchestrate follow` to apply on a
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

## Follow-through preset

`hev orchestrate follow` (formerly `hev expedite`, still a deprecated alias;
see [follow-through orchestrators](orchestrate-follow.md)) uses the fixed snapshot run names and `say, rework, notify,
validate` authority. Read-only review of exact heads with `gh pr view/diff/checks`
and reading run reports/worktrees is always permitted. Independent verification
uses `hev dispatch validate NAME --host HOST` (HOST is the followed run host in
an orchestrator round), with the configured different validator harness/model.
The next round reads validation.json and verdict.json, requiring the current
revision; unavailable tools/hosts/models produce one `[needs you]` reply until
evidence changes. The preset never grants merge authority; the documented
legacy `merge-on-green` helper retains its separate reviewed-head gate.

Before starting, the preset registers admission-only copies on every snapshot
host and the validator host using `orchestrator-apply` with a scope owner. They
cannot supervise. Any refusal aborts startup and names the host. Stop removes
registrations. Old hosts must support the admission-only registration handshake;
they safely refuse startup rather than silently starting an extra supervisor.

With `idle: skip`, unchanged evidence and no inbox messages cost no model turn.
After three unchanged polls with the accepted checkpoint and named waits intact,
the supervisor sends one stalled notice containing the first `[needs you]` line
and reports `stalled`. It polls run state and PR head/checks without a harness
until evidence or inbox changes. Status includes the stall reason and pending
notice count, newest message and error.

Native owner notices share peers' incarnation-bound beacon relay for cached
hosts, including historical laptop aliases. Enqueue is queued, not delivered;
the recipient inbox receipt completes delivery. With no cached host, unresolvable SSH names fall back to pull. Stale snapshots
and absent recipients remain queued and recheck the local relay each flush.
Transient SSH failures retain backoff. Previously pull-routed transient failures
resume retries automatically.
`hev dispatch notices pull --host HOST --deliver` delivers orchestrator notices
as well as member notices, before acknowledging stable IDs. Run it on the owner
laptop for each remote host; the orchestrator-only pull command is unchanged.

### Round harness fallback

`hev orchestrate start` defaults to `agy` with `gemini-4-neon-medium`.
Selecting another harness alone uses its native default model. Handwritten
specs default to the `agy` harness. Omitting `model:` in a handwritten spec
always leaves model selection to the harness. Set an ordered list in spec frontmatter, for example:

```yaml
fallbacks:
  - harness: codex
    model: gpt-6.2-astra
    effort: high
  - harness: claude
```

When the list is omitted, apply uses `[[orchestrate.fallbacks]]` from the
target's dispatch config, then top-level `[[fallbacks]]`. Both accept the same
three fields. An explicit empty list disables fallback. A launch error,
nonzero exit, or missing or blank checkpoint advances to the next candidate
within the round deadline. Stop and halt suppress retries. Each round keeps
`attempts.json`, the final `trace`, and prior `trace.attempt-N` files.

## Wrapper templates

Use `hev orchestrate template swarm`, `hev orchestrate template climb` or
`hev orchestrate template swarm-climb` to print a TOML starter. Save it as
a `.toml` file and edit the goal, reply address and worker orders. Preview
the compiled swarm with `hev orchestrate apply -f FILE.toml --dry-run`.
Apply it without `--dry-run` to start the swarm supervisor. The swarm
template uses the local supervisor in the unscoped default workspace.

A swarm template sets `kind = "swarm"` or declares `[[roles]]`. Put the
Markdown problem and role sections in the top-level `brief` string.
Settings such as `every`, `max_lifetime` and `[budget]` go alongside
the role tables. Markdown orchestrator and swarm files remain supported.

A `[climb]` table declares the across-generation loop. Add `[[roles]]` to
declare fan-out and reconciliation within each generation. After validating
a climb template, apply and dry-run report that the climb runtime
(PRs #322/#323) is required. A combined template never launches just the swarm.
