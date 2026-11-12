# RFC 0013: Swarms — one file that puts an orchestrator and its workers on a problem across hosts

Status: Phase 1 implemented (2026-11-12): a single-host swarm, see
[docs/swarms.md](../swarms.md) and "Phase 1 as built" below. Phases 2–5 are
design only. Proposed 2026-11-11.

## Summary

A swarm is one Markdown file that declares a problem, one orchestrator and a
set of worker roles with concurrency limits. `hev swarm up -f FILE` compiles it
into what kit already runs: an RFC 0008 orchestrator and ordinary dispatch
runs. The swarm adds three things the pieces lack today: **desired state**
(which units of work exist and how many may run at once), a **reconcile
step in code** inside the orchestrator's supervisor that holds that state, and
**fencing** (generation-numbered runs on separate branches, plus an
epoch-numbered owner lease) so that replacing a worker or an orchestrator can
never leave two of them acting on the same work.

A swarm is a thin layer. Hosts keep admission and capacity. Workers stay
worktrees and harness processes. The orchestrator still writes no code and
merges nothing. Phase 1 works on one host with no new state outside
`~/dispatch/`; cross-host placement and orchestrator takeover come later and
each phase is one dispatchable order.

### The decision in five lines

1. A swarm file is a kit-native, compose-like Markdown file. It is not a Docker Compose file. Its YAML frontmatter extends the orchestrator spec with `roles:`, and each role's fields are order frontmatter keys.
2. `hev swarm up|ps|logs|scale|say|pause|resume|down|sweep` drive `hev orchestrate` and `hev dispatch` underneath. Swarms add no third runtime.
3. Two shapes: `partition` (one run per piece, then an `integrate` worker produces the single draft PR) and `race` (variants of one task; the orchestrator picks one, a person merges).
4. The reconciler is code inside the owning orchestrator supervisor and acts only while that supervisor holds the swarm lease. `hevd`'s existing scan, or a scheduled `hev swarm sweep`, only restarts a dead supervisor or claims an expired lease.
5. A worker is replaced automatically only when its role is `effects: branch`, the old run is confirmed stopped or its host has been unreachable past the grace period, and the replacement is fenced to a new branch. Anything else stops and asks a person.

## Motivation

- **Swarming is hand assembly today.** Putting several agents on one problem
  means writing an orchestrator spec, then N orders by hand, then applying
  scope-only registrations on each remote target
  (`docs/orchestrators.md`, "Remote targets register the same spec"). Nothing
  records that these runs belong together, so `ls`, budgets and cleanup all
  treat them as unrelated.
- **Orchestrators react; they don't hold a state.** An RFC 0008 round can
  notice a dead run and call `retry`, but only through a model round, at the
  `every` cadence, with no restart limit, and with no way to say "three of
  these should be running". Replacing a crashed worker should not cost a model
  call.
- **Nothing survives losing a host.** Orchestrator state lives in
  `~/dispatch/orch-NAME/` on one host (`docs/orchestrators.md`). RFC 0008 says
  "Restart/reboot recovery remains outside this change". A run on a host that
  drops is shown as unreachable (`cmd/hev/dispatch.go`) and stays that way until
  a person acts.
- **The rules that make dispatch safe also block healing.** Dispatch says an
  uncertain outcome is inspected before a retry and that you "never start a
  second worker on a guess" (`docs/dispatch.md`, Amend). Self-healing has to
  keep that promise rather than quietly drop it. This RFC keeps it by making a
  second worker harmless (fencing) instead of assuming the first one stopped.

## Decision

Add `hev swarm` and a swarm file. `up` validates the file, writes an
orchestrator spec and per-role order templates into the orchestrator's state
directory, and applies them. The supervisor gains a code-only **reconcile
tick** that runs between rounds, compares desired state with dispatch
inventory, and repairs what code can repair safely. Judgement (splitting the
problem, choosing a race winner, answering a blocked worker, deciding about
orphaned commits) goes to orchestrator rounds and then to the person in
`reply`. Swarms are off by default and limited by run, time, restart and plan
budgets.

## Goals

- One file declares the problem, the orchestrator and the worker roles.
- One vocabulary: the orchestrator block uses RFC 0008's field names and role
  fields use order frontmatter keys (`schemas/orchestrator.schema.json`,
  `schemas/order.schema.json`).
- Useful on one host. Cross-host placement and orchestrator takeover are
  additions, not prerequisites.
- Self-healing for the failures code can judge, with explicit limits on when
  it stops and asks.
- At most one live, acting worker per unit of work and at most one acting
  orchestrator per swarm, including across partitions.
- Default off: nothing new runs on a host that has not applied a swarm or
  registered as its standby.

## Non-goals

- A central scheduler, a control plane, a consensus cluster or a new listener.
- Containers. Workers are worktrees and harness processes (`docs/dispatch.md`).
- Overriding host admission or capacity. The swarm asks and the host decides
  (`MISSION.md`).
- Merge authority. The swarm's output is one draft PR (partition) or one
  chosen branch (race), and a person merges.
- Sandboxing. The cooperative trust boundary of RFC 0008 and RFC 0011
  is unchanged.
- Swarms inside swarms.

## Vocabulary

- **Swarm**: a named orchestrator plus its declared worker roles and their
  desired state.
- **Role**: a kind of worker: order fields, a brief, a shape and a concurrency
  limit (`replicas`).
- **Unit**: one piece of work a role owns: a partition *piece* or a race
  *variant*. Each unit has at most one current run.
- **Generation**: a counter per unit. A replacement on another host gets the
  next generation, a new run name and a new branch. Earlier generations are
  *fenced*.
- **Reconcile tick**: one code-only pass comparing desired state with observed
  dispatch inventory and taking safe repairs.
- **Lease** and **epoch**: the right to act as the swarm's owner, held by one
  host at a time. The epoch rises on every takeover.
- **Standby**: a host registered to take the lease if the owner is lost.

## What kit already has

| Piece | Where | Reused as |
|---|---|---|
| Orchestrator spec with nested YAML, strict known fields | `internal/dispatch/orchestrator_spec.go`, `schemas/orchestrator.schema.json` | The swarm file's decoder and the orchestrator block |
| Flat order frontmatter and `## Acceptance` | `internal/dispatch/order.go`, `schemas/order.schema.json` | Role fields; compiled worker orders stay flat and valid |
| Rounds, checkpoints, required checkpoint, failure streak | `internal/dispatch/orchestrator.go`, `docs/rfcs/0008-orchestrators.md` | Judgement steps |
| Scoped authority (`say`, `notify`, `rework`, `retry`, `validate`, `dispatch`) | `internal/dispatch/orchestrator_spec.go`, `internal/dispatch/orchestrator_admission.go` | Limits what both the rounds and the reconcile tick may do |
| Scope-only registration on remote targets | `docs/orchestrators.md`, `cmd/hev/orchestrate.go` (`--scope-only-owner`) | Registering worker hosts |
| Run states `done`, `failed`, `timeout`, `died`, `blocked`, `needs-review` | `internal/dispatch/run.go`, `docs/dispatch.md` | Detecting worker exits |
| Supervisor-owned timeout; stop kills the recorded process group, even after the supervisor died | `internal/dispatch/process_unix.go`, `internal/dispatch/process_windows.go`, `docs/dispatch.md` | Hang repair and confirming a dead run is stopped |
| `rework` reuses worktree and branch | `docs/dispatch.md` | In-place repair that keeps unpushed work |
| Receiver FIFO queue and `hev dispatch sweep` | `internal/dispatch/queue.go`, `docs/rfcs/0012-dispatch-queue.md` | Full hosts |
| hevd scan cycle already sweeps the queue | `internal/daemon/daemon.go` (`SweepQueueBackstop`) | Supervisor watchdog and lease claim |
| Repository admission on the target | `internal/dispatch/admission.go`, `docs/dispatch.md` | Placement candidates |
| Class caps | `internal/dispatch/classes.go` | Per-role caps on hosts |
| Host move with forwarding | `internal/dispatch/amend_transfer.go` | Not used for healing; it needs the source host reachable |
| `attempting` / `live` / `needs-reconcile` receipts for uncertain launches | `internal/dispatch/orchestrator_source.go`, RFC 0008 Phase 2 | The unit record's launch states |
| Bound senders; orchestrators fail closed for them | `docs/rfcs/0011-nodes.md`, `docs/nodes.md` | Trust across hosts |
| Durable shared findings | `docs/board.md` | The swarm's findings thread |
| Run inbox and native peers addresses `HOST/NAME`, `HOST/o-NAME` | `docs/peers.md` | Talking to workers and the orchestrator |
| Repeating-run supervisor that never overlaps rounds | `internal/dispatch/repeat.go` | Unchanged; the round cadence |

Plan meters come from `hev subscriptions` (PR #204). This RFC assumes it
lands. Nothing below cites a file from it.

## Proposed surface

### 1. One file

**Decision: compose-like and kit-native. Kept from the sketch, with two
corrections.**

The format is the orchestrator spec format: Markdown with YAML frontmatter
decoded strictly. The body is the shared brief. The sketch said kit's
frontmatter rejects nesting. That is true only of *orders*: `order.go` parses
flat `key: value` lines and rejects `[]{}`. Orchestrator specs already decode
nested YAML with `KnownFields(true)` (`orchestrator_spec.go`), so a swarm needs
no new format. The sketch also listed `acceptance` among order frontmatter
fields. Acceptance is a body section (`## Acceptance`), not a key, so a role's
acceptance is a body section too.

```yaml
kind: swarm
name: logfix                 # orchestrator o-logfix; run names start logfix-
goal: Replace the deprecated logging calls in app with the structured logger.
reply: owner-session
# --- RFC 0008 orchestrator fields, same names and meanings ---
host: build-1                # owner (where up runs)
every: 30m                   # judgement round cadence
max_lifetime: 2d
harness: claude
model: claude-opus-5-5
round_timeout: 30m
budget: {concurrent_runs: 5, runs_per_day: 30, usd_per_day: 0}
# --- swarm additions ---
standby: [build-2]           # may take over the lease if the owner is lost
reconcile_every: 2m          # code-only tick; default 2m
host_grace: 10m              # unreachable this long = unit lost
restart: {limit: 3, backoff: 2m, max_backoff: 30m, swarm_limit: 10}
plans: {hold_at: 85, shrink_at: 95}   # percent of a plan meter
max_runs: 40                 # lifetime total, all roles, all generations
roles:
  - name: migrate
    shape: partition
    replicas: 3              # at most 3 pieces running at once
    pieces: [api, worker, cli, storage, web]   # or: pieces: orchestrator
    effects: branch          # default; see Self healing
    stall: 45m
    order: {repo: app, base: main, harness: codex, model: model-a, timeout: 90, class: code}
    placement: {spread: true, os: linux}
  - name: integrate
    shape: single
    after: [migrate]
    effects: pr              # opens the one draft PR; never auto-replaced cross-host
    order: {repo: app, base: main, harness: claude, model: claude-opus-5-5, timeout: 60}
    placement: {hosts: [build-1]}
```

The body holds `## Problem`, a swarm-level `## Acceptance`, and one
`## Role: NAME` section per role with its own `### Acceptance`. `up` compiles,
for each unit, an ordinary order: the role's `order` keys become flat
frontmatter plus `name`, `host`, `reply`; the body is Problem + Role section +
the unit assignment (`Piece: storage`) + the role's acceptance as
`## Acceptance`. The compiled orders pass today's `dispatch.Parse` unchanged,
so the order format does not change.

`order` accepts exactly the order keys except `name`, `host`, `reply` and the
repeat keys (`every`, `until`, `max_lifetime`), which the swarm owns. A typo is
an error, as in both existing parsers.

**A literal Docker Compose file would buy** familiar syntax, existing editor
schemas, and `deploy.replicas`, `deploy.placement.constraints` and
`restart_policy`, which map closely. **It would cost** the meaning of almost
everything else: `image`, `ports`, `volumes`, `networks`, `depends_on`
conditions and health checks describe containers. Every kit field would live
under `x-hev:` extensions, which strict decoding cannot check. `docker compose
up` on the file would try to start containers. And the file would stop being
the order vocabulary operators already write. Kit borrows Compose's verbs and
three field ideas, not its file.

### 2. Lifecycle verbs

**Decision: `hev swarm VERB`, a noun group like `hev orchestrate` and `hev
dispatch`. Every verb resolves to existing verbs plus the swarm's state file.
Kept from the sketch.**

| Verb | What it does | Underneath |
|---|---|---|
| `hev swarm up -f FILE [--dry-run]` | Validate, compile, apply. Rerunning with an edited file updates desired state in place, as in Compose. | `orchestrate apply` on the owner; `orchestrate apply --scope-only-owner` on each candidate host (phase 3); the first reconcile tick launches units with dispatch `start`. |
| `hev swarm ps NAME [--json]` | Units by role: piece or variant, generation, host, state, branch head, restarts, fenced generations | `orchestrate show` + `dispatch ls --json` rows whose `ORCH` is the swarm |
| `hev swarm logs NAME [ROLE[/UNIT]] [-f] [--round N]` | Without a unit: orchestrator rounds. With one: the current generation's run log | `orchestrate logs`, `dispatch logs` |
| `hev swarm scale NAME ROLE=N` | Change `replicas` in desired state | Owner state write; the next tick acts |
| `hev swarm say NAME [ROLE/UNIT] MSG` | Message the orchestrator, or one unit's current run | `orchestrate say`, `dispatch say` |
| `hev swarm pause|resume NAME` | Stop or restart launches, repairs and rounds. Running workers continue. | `orchestrate pause/resume` |
| `hev swarm down NAME [--keep-runs]` | Stop the orchestrator, then stop every current run. Worktrees, branches and PRs stay. Prints what is left. | `orchestrate stop`, `dispatch stop` per run |
| `hev swarm sweep` | Restart dead supervisors and claim expired leases for swarms registered on this host | Same entry point as `hev dispatch sweep` |

`scale` down never stops a running unit. It lowers the limit, cancels queued
units first (cancelling a queued order is already atomic, RFC 0012), and lets
running ones finish. `--force` stops the newest running units through the
`stop` authority and records them as `held`, never `done`.

`down` is final for that lifetime, like `orchestrate stop`. Running `up` again
on the same file starts a new lifetime and keeps unit records, generations and
round numbering, as reapplying a stopped orchestrator does today
(`docs/orchestrators.md`).

### 3. How a swarm works a problem

**Decision: the file expresses `partition`, `race` and `single` roles, plus
`after` ordering. Integration is a worker role. RFC 0008's "never writes
code, no merge authority" is kept unchanged.**

**Partition.** Pieces come from the file (`pieces: [...]`) or from the
orchestrator (`pieces: orchestrator`). In the second case the first round must
write `pieces.json` beside its checkpoint. Splitting the problem is judgement.
Code validates the piece names, count and `max_runs`. Each piece gets its own
run, `SWARM-ROLE-PIECE-gN`, so a unit, its branch and its fence are one thing.
`replicas` limits how many run at once. The tick launches the next unassigned
piece when a slot frees. A piece is complete when its run ends `done` with a
pushed head.

**Race.** `variants: [{model: model-a}, {harness: claude, model: model-b}]`
runs the same order once per variant (`SWARM-ROLE-v1-g1`, ...). `choose:` says
who picks:

- `first-green` (code): the first variant whose run is `done` and whose
  validation or CI is green. The tick stops the others with `stop`.
- `orchestrator` (default): a round compares reports and branches, records
  the choice and its reason in the checkpoint, and sends it to `reply`. The
  round also writes `choice.json` (`{"role","unit","reason"}`) in its round
  directory; code records it after the checkpoint is accepted.
- `person`: the round only summarises; `hev swarm say NAME choose v2` records
  the choice.

Losing branches are kept and never deleted.

**Integration.** A `single` role with `after: [migrate]` launches once every
piece is complete. Its order lists the piece branches (current generations
only). It merges them onto its own branch, resolves conflicts, runs the tests
and opens the swarm's one draft PR. If a piece is reworked later, the tick
`say`s the integrate run, or reworks it if it has ended, with the new head.
Integration is code, so a worker does it. The orchestrator judges whether
the goal is met and a person merges, exactly as in RFC 0008 ("reports
merge-ready to `reply`").

**PRs.** Compiled worker orders for `effects: branch` roles say "commit and
push your branch; do not open a PR". Explicit order instructions already
override the worker prompt's defaults (`docs/dispatch.md`). Only the
`effects: pr` role opens a PR, so a swarm produces one PR, not one per piece.

**Shared findings.** When the board is configured, `up` creates one thread
(`hev board post --board swarms --title NAME`). Every compiled order tells the
worker to reply there with findings other units need, and rounds read it into
the packet. The board is durable and searchable (`docs/board.md`). Without a
board, findings go to the orchestrator inbox with `hev dispatch say o-NAME`.
That already routes to the owner and wakes it (RFC 0008 Phase 2). Peers group
chats are not used: the native engine that dispatch runs use does not carry
them (`docs/peers.md`, Group chats).

**Who decides the result.** Code decides piece completion and `first-green`.
The orchestrator decides the split, race choice (by default) and goal status.
A person decides merge and anything listed under Self healing as "asks".

### 4. Cross host

**Decision: the swarm proposes a host per unit and the host's admission
decides. The orchestrator runs on its owner host with optional standbys. The
`host` field stays a label for the owner, and standby registration is the one
new way a supervisor can start on another host.**

Placement for each launch, in code:

1. **Candidates**: the owner's configured dispatch hosts (`dispatch.toml
   [hosts.*]`, `docs/dispatch.md`), filtered by the role's `placement`:
   `hosts: [...]` (pin), `os: linux|darwin|windows`, `harness` available
   (`hev dispatch hosts --json` reports versions; harness presence becomes a
   reported field in phase 3), and `exclude: [...]`.
2. **Repository admission**: a host that refused the role's repo (`Error:`
   naming repo and host, `docs/dispatch.md`) is dropped for that repo for the
   lifetime, and rechecked on `up`. Hosts where the owner is a bound sender
   are dropped too (see Trust).
3. **Order**: with `spread: true`, fewest current units of this role first,
   then most free slots, then name. That is today's `dispatch run` rule with
   spread added. Without spread, most free slots.
4. **Full everywhere**: enqueue on the first-ranked host's FIFO (RFC 0012).
   The unit is `queued` and holds no class slot. If another candidate frees
   first, the tick may cancel the queued order (atomic `stop` on a queued
   order) and start the unit there. Cancelling a queued order has no effects,
   so this is safe.
5. **Admission says no** (cap, class, policy): the unit stays `pending` with
   the reason. The swarm never retries a refusal faster than the backoff and
   never raises a host's cap.

The orchestrator runs on the host where `up` ran, as RFC 0008 requires
("apply/start must run on the intended supervisor host",
`docs/orchestrators.md`). Same-host evidence stays cheapest, so `placement`
defaults let workers land on the owner too. `standby: [HOST...]` registers the
spec on those hosts in a new standby mode: no rounds, no reconcile, only a
lease check (see 6). **This amends RFC 0008 Placement**: a standby
registration is the operator's advance consent for a supervisor to start on
that host when the lease expires, without a person running `apply` there.

Worker hosts get the same scope-only registration as today. **This amends
RFC 0008's rule "Owner unavailability fails admission"** for swarms. A target
registers the owner set (owner plus standbys) and the highest epoch it has
seen, and accepts a swarm operation only from a member of that set presenting
an epoch at least that high. An operation with a higher epoch raises the
recorded one under the dispatch root lock. A stale owner is refused by every
target the new owner has reached.

### 5. Self healing

**Decision: desired state is the unit table. The tick repairs deterministic
failures. Rounds handle judgement. Automatic replacement happens only under
the conditions below, and any other case asks.**

**Desired state** lives in `swarm.json` in the orchestrator state directory.
It holds the roles and their `replicas`, the unit table, and the restart and
plan budgets. One unit record looks like this:

```json
{"role":"migrate","unit":"storage","gen":2,"run":"logfix-migrate-storage-g2",
 "host":"build-3","state":"live","base":"d/logfix-migrate-storage-g1",
 "head":"4f1c...","restarts":1,"next_try":"...",
 "fenced":[{"gen":1,"run":"logfix-migrate-storage-g1","host":"build-2","at":"...","head":"9ab2..."}]}
```

Unit states are `pending`, `attempting`, `queued`, `live`, `done`, `failed`,
`lost`, `held` and `superseded`. **The record is written before every launch**
(`attempting`), following the 0008 source receipt pattern. An uncertain launch
stays `attempting`. It becomes `live` when its run appears in inventory and
`needs-reconcile` (a kind of `held`) after a 15-minute grace. The tick never
relaunches the same generation, so a lost acknowledgement cannot make two runs
under one name. *Phase 1 amendment:* on a single host the target always
answers, and it refuses a duplicate run name atomically under its root lock.
So an `attempting` unit with no run in inventory gets the identical start
again: if the first landed the second is refused, and the next tick sees the
run. The 15-minute `needs-reconcile` grace applies from phase 3, when the
target may be unreachable.

**The reconcile tick** (code, no model, every `reconcile_every`, only while
holding the lease):

1. Renew the lease (phase 4) or stop acting.
2. Gather `dispatch ls --json` from the owner and every host with a unit, in
   parallel, with the existing per-host unreachable handling.
3. For each unit, compare observed with desired and take the repair from the
   table below, subject to budgets and backoff.
4. Launch pending units up to `replicas`, the budget and plan limits.
5. Write `swarm.json`. Wake a round only if something needs judgement.

**When automatic replacement is safe.** All of the following must hold:

- The role is `effects: branch` (the default). Its compiled order allows only
  commits and pushes to its own `d/RUN` branch, and forbids PRs, comments,
  deploys, messages to people and any other outward effect. This is a
  cooperative instruction, like every worker rule today.
- The old run is **confirmed stopped** (its host answers and reports a
  terminal state after a `stop`), **or** its host has been unreachable for
  `host_grace` and the old generation is fenced *before* the new one launches.
- The unit's restart count is under `restart.limit`, the swarm's total is
  under `restart.swarm_limit`, and the backoff has elapsed.
- The old run did not end `blocked` or `needs-review`. Those are questions,
  not crashes.

**This amends a dispatch rule.** `docs/dispatch.md` says to inspect an
uncertain transfer and "never start a second worker on a guess". Swarms keep
the inspection and change what follows it. The tick reads the record, stops
through the recorded process group, and confirms the terminal state. Where it
cannot confirm (host lost), it does not guess that the old worker stopped. It
assumes the old worker may still be running and makes that harmless: the old
generation can write only to its own worktree and its own branch, and nothing
downstream reads a fenced branch. `effects: pr` and any role with outward
effects keep today's rule unchanged.

**Repairs in order of preference:**

1. **Same host, host reachable, run ended `failed`/`timeout`/`died`.** Run
   `stop` first; it uses the recorded process group even if the supervisor
   died (`docs/dispatch.md`). Then `show` must report a terminal state. Then
   `retry` or `rework` in place, with a note naming the failure. This keeps
   the worktree, the unpushed work and the branch, and does not change the
   generation.
2. **Different host, needed when the old host refuses or has lost the unit.**
   Start generation N+1 on another candidate. Its `base` is the fenced
   generation's last **pushed** head (its `d/RUN` branch), or the role's base
   if nothing was pushed. Its order adds a `## Predecessor` section: the old
   run name, host, last status lines if readable, pushed head, and the reason.

**Unpushed work on a lost host** is not recoverable while the host is gone. The
replacement starts from the last pushed head, and compiled `effects: branch`
orders say "push after every commit" to bound the loss to one step. When the
host returns, the tick stops the fenced run and leaves its worktree in place.
If the fenced branch or worktree holds commits beyond the head the
replacement started from, the next round gets an `orphaned-work` item: run,
host, commit count and range. The round decides whether to tell the current
generation to cherry-pick them (`say`). Code never merges or deletes them.

#### Failure table

| Failure | Detection (code) | Repair (code) | Stops and asks when |
|---|---|---|---|
| **Worker harness exits** | Run state `failed`, `timeout` or `died` in `dispatch ls`. `done` without a pushed head is treated as `failed`. | Repair 1 (in place) after backoff | Restart limit reached (unit `held`); the run ended `blocked` or `needs-review` (forwarded to a round, then `reply`); `stop` cannot confirm a terminal state; role is `effects: pr` and the failure is anything but a clean `failed` before its PR existed |
| **Worker hangs** | Supervisor `timeout` (the role's `timeout`); or `stall`: no new log bytes, status line or pushed commit for `stall` | `stop` (kills the process tree), confirm terminal, then repair 1 | Same as above; a second stall on the same unit is judged by a round before a third attempt |
| **Worker's host unreachable** | Host unreachable in every tick for `host_grace` | Mark unit `lost`, fence generation N in `swarm.json`, then repair 2 on another candidate | No other admitted candidate (unit `held`, swarm keeps going); role not `effects: branch`; unit already lost once on another host within `host_grace` × 3 (looks like a partition, not a crash) |
| **Lost host returns with the old worker still running** | A fenced run appears `live` in inventory | `stop` the fenced run; if it ended on its own, nothing to stop. Record its head; raise `orphaned-work` if it has commits past the replacement's base. Mark it `superseded`. | `stop` fails; the fenced run opened a PR or did anything visible outside its branch (an `effects: branch` violation, reported, never auto-repaired) |
| **Orchestrator round fails** | Existing: no accepted checkpoint, failure streak (`consecutive_failures`) | Previous checkpoint stands (existing). The tick keeps doing deterministic repairs. No new judgement: no new pieces, no race choice, no answers. | Existing notice at the second consecutive failure. At `round_failures_limit` (default 3) the swarm **holds**: no new launches and no cross-host replacement, only in-place repair 1. |
| **Orchestrator supervisor dies** | `hevd` scan or `hev swarm sweep` on the owner: state says running but no `o-NAME` supervisor (Unix tmux session; Windows PID and start time, as dispatch checks) | Stop any orphaned round harness through its recorded process group, discard the round without a checkpoint (the previous one stands, as today), relaunch the supervisor | Supervisor restarted 3 times in an hour; lease held by another host (then the local one stays down) |
| **Orchestrator's host lost** | Standby's sweep sees the lease past expiry plus margin | Standby claims the lease with epoch + 1 (an atomic compare-and-swap on the state ref), reads `checkpoint.md` and `swarm.json` from the ref, announces the epoch to worker hosts, starts its supervisor | No standby configured (the swarm keeps whatever is running and holds; workers finish within their timeouts); the claim races and loses (the other claimant owns it); the state ref is unreachable |
| **Partition between hosts** | Unreachable from the owner but not necessarily down | Workers on the far side keep going on their own branches. After `host_grace`, repair 2 as for a lost host. The owner, if cut off from the state ref, stops acting once its lease lapses. | Any case where the lease cannot be renewed or claimed by anyone: the swarm holds until someone can reach the ref |

### 6. Where the reconciler lives

**Decision: in the owning orchestrator supervisor, gated by a lease kept in a
compare-and-swap ref on the swarm repository's origin. `hevd`'s scan and
`hev swarm sweep` act only as a watchdog. Default off holds: only hosts that
applied a swarm or registered a standby do anything.**

Options considered:

- **(a) Inside the orchestrator supervisor.** No new process. It is already
  per-swarm, visible as `o-NAME`, and lives exactly as long as the swarm. It
  dies with its host, so alone it cannot heal the orchestrator, and it needs
  something to restart it after a crash.
- **(b) In `hevd`'s scan cycle.** It survives supervisor crashes and the scan
  already sweeps the dispatch queue (`internal/daemon/daemon.go`). But it
  would make hevd a scheduler for every swarm on the host. hevd is a capture
  daemon that is optional and may not run, and its scan interval is for
  indexing, not repair.
- **(c) A lease in shared state.** This is the only option that gives one
  acting owner across hosts without a quorum service. It needs shared state
  that every host can reach and that supports compare-and-swap. Kit has none
  of its own, but every swarm already has one: the repository's origin, which
  every worker fetches and pushes. `git push --force-with-lease=REF:EXPECTED`
  is atomic on the forge.

**Recommended: (a) + (c), with (b) as watchdog.** The reconcile tick runs in
the supervisor and acts only while that host holds the lease. The lease is a
branch `hev-swarm/NAME` on the origin of the swarm's first `manages.repos`
entry, or `state_repo:` to use a private repository. It holds one orphan
commit with `lease.json` (`holder`, `epoch`, `renewed`, `ttl`), `swarm.json`,
`checkpoint.md` and `spec.md`. Each tick replaces that commit with a
compare-and-swap push. Round directories stay local because the checkpoint is
what carries the work (RFC 0008).

Rules:

- The owner acts (tick repairs, rounds, admissions) only while
  `last successful renew + ttl − margin > now`. Defaults: `ttl` 10m, `margin`
  2m, renew each tick.
- A standby claims only when `renewed + ttl + margin < now` by its clock, with
  a compare-and-swap from the exact commit it read. Two standbys racing are
  settled by the forge: one push wins.
- A returning old owner fails its next compare-and-swap. Before that, its
  lease has already lapsed by its own clock, so it has stopped acting. Its
  supervisor exits with `lease-lost`, notifies `reply`, and leaves its state
  directory for inspection.
- Targets refuse operations with a stale epoch (section 4). Fencing therefore
  does not depend only on clocks. A stale owner that missed both its own
  lapse and a target's epoch could still act on a target the new owner has
  not reached yet. The first thing a new owner does is announce its epoch to
  every host in the unit table.
- Assumption: clock skew between hosts is below `margin`. `up` refuses a
  `margin` below 1m.

**Two orchestrators at once** are prevented by the lease (one holder per
epoch), by targets' epoch checks, and by RFC 0008's existing lock and
"no overlapping rounds" on a single host. **Two replacement workers for one
unit** are prevented four ways. Only the lease holder launches. A unit
generation is written `attempting` before launch and never relaunched. Run
names include the generation, so the target refuses duplicate names
atomically. And integration reads only the current generation's branch, so
even a worker that is still running cannot have its output used.

Phase 1 needs none of this: on one host, the state stays local in
`swarm.json`, there is no ref and no lease, and the RFC 0008 root lock is
enough.

The watchdog, as one function on two schedules: hevd's scan calls it after
`SweepQueueBackstop`, and `hev swarm sweep` runs it from a scheduled task. It
does two things, only for swarms registered on this host: restart a dead
supervisor that still holds the lease, and try a claim if this host is a
standby and the lease has expired. It never reconciles units itself.

### 7. Bounds

**Decision: every limit is in the file, has a default, and when hit holds
the swarm rather than looping.**

| Bound | Default | When hit |
|---|---|---|
| `budget.concurrent_runs`, `runs_per_day` (RFC 0008) | from the file; `up` requires a positive `concurrent_runs` | Existing admission refusal and daily notice; units wait `pending` |
| `max_runs` (lifetime, all generations and rework attempts) | 4 × total replicas | Swarm holds; asks |
| `max_lifetime` (RFC 0008) | required | Swarm ends with "budget exhausted", not "goal met", as today |
| `replicas` per role | ceiling 8 | `up` and `scale` refuse above it |
| Units per swarm | ceiling 32 | Refused at `up` or when the orchestrator writes `pieces.json` |
| `restart.limit` per unit | 3 | Unit `held`; asks |
| `restart.swarm_limit` | 10 | Swarm holds; asks |
| Backoff | 2m, doubling, capped at `max_backoff` 30m | — |
| `round_failures_limit` | 3 | Swarm holds (see failure table) |
| `usd_per_day`, `usd_total` | informational | Recorded only. Enforcement waits for cost accounting, as `docs/orchestrators.md` says of `usd_per_day` today. |
| `plans.hold_at` | 85 | No new launches or cross-host replacements; in-place repair continues |
| `plans.shrink_at` | 95 | Also cancel queued units and do not refill slots. Running workers are never stopped for plan reasons. |

Plan meters come from `hev subscriptions --json` (PR #204). Each tick reads
it once on the owner and, from phase 3, once on each host with a unit. A
unit's meter is the one for its harness on its host. An unreadable meter is
treated as "unknown", which holds new launches for that harness and host
only. **Hold** is reversible: when the meter drops, launches resume without a
person.

### 8. Trust

**Decision: a swarm may do a few things an order or a plain orchestrator
cannot, all inside its own scope. Every RFC 0008 and RFC 0011 check still
applies on each host.**

What a swarm adds beyond an order: start up to `max_runs` runs, stop its own
runs, replace runs on other hosts, write one state branch on origin, and
(standby) start a supervisor on another host.

What it may not do: touch runs that are not its own, merge, raise a host's
cap or class caps, bypass repository admission, run on a host where its owner
is only an invited teammate, or nest another swarm.

- **Scope.** A new selector `swarm=NAME` matches runs whose metadata records
  `swarm: NAME`. Admission writes that from the orchestrator identity on
  `start`; a caller cannot claim it. The compiled spec's `manages.runs` is
  this selector, and `manages.repos` lists the roles' repos.
- **Authority.** A swarm's orchestrator gets `say`, `notify`, `rework`,
  `retry`, `dispatch` and a new **`stop`** verb, scoped like the others
  (`orchestrator_spec.go` accepts only the first six today). **This amends
  RFC 0008's authority list.** `stop` is needed for hang repair, fencing,
  race losers and `scale --force`. `validate` is included only if a role sets
  `validation: browser`. There is no `merge`.
- **The tick's identity.** The reconcile tick calls dispatch with
  `HEV_ORCHESTRATOR=NAME`, the same identity a round uses, so target
  admission applies the same scope, authority and budget checks to code and
  model alike.
- **Nesting.** The swarm's orchestrator counts against RFC 0008's depth cap
  of 2. A plain orchestrator may manage a swarm with `allow_nested: true`. A
  swarm may not contain a swarm.
- **Bound senders (RFC 0011).** Invited teammates' RPC fails closed for
  orchestrators, and that is kept. A teammate cannot `up` a swarm on a shared
  host, and a swarm owner that is only a bound sender on some host does not
  place units there. The worker's `requested_by` is the swarm owner's
  identity, recorded by the owner's host as today. Opening swarms to bound
  senders would need per-grant authority, which RFC 0011 leaves to a future
  version.
- **Across hosts.** RFC 0008's trust boundary ("identity, not
  authentication") is unchanged: epochs and owner sets constrain cooperative
  supervisors and do not authenticate a hostile caller. The state ref adds one
  exposure: anyone with read access to the repository can read the
  checkpoint. Use `state_repo` for a private one.

## Example: end to end

A generic library `app` uses a deprecated logging API in five packages. Three
hosts are configured for dispatch (`build-1`, `build-2`, `build-3`), and all
three admit `app`.

`logfix.md`:

```markdown
---
kind: swarm
name: logfix
goal: Replace every deprecated logging call in app with the structured logger, tests green, one draft PR.
reply: owner-session
host: build-1
standby: [build-2]
every: 30m
max_lifetime: 2d
harness: claude
model: claude-opus-5-5
budget: {concurrent_runs: 4, runs_per_day: 20}
reconcile_every: 2m
host_grace: 10m
restart: {limit: 3, backoff: 2m, max_backoff: 30m, swarm_limit: 8}
plans: {hold_at: 85, shrink_at: 95}
max_runs: 24
roles:
  - name: migrate
    shape: partition
    replicas: 3
    pieces: [api, worker, cli, storage, web]
    stall: 45m
    order: {repo: app, base: main, harness: codex, model: model-a, timeout: 90, class: code}
    placement: {spread: true, os: linux}
  - name: integrate
    shape: single
    after: [migrate]
    effects: pr
    order: {repo: app, base: main, harness: claude, model: claude-opus-5-5, timeout: 60}
    placement: {hosts: [build-1]}
---
## Problem

`app` still calls `log.Printf` and the old `logx` helpers in five packages.
Move each call to `slog` with structured fields. Keep messages and levels.

## Acceptance
- No call to the deprecated API remains (`grep` proof in the report).
- `go test ./...` passes on the integration branch.
- One draft PR against `main`.

## Role: migrate

Convert only your piece's package and its tests. Push after every commit.

### Acceptance
- Your package has no deprecated calls; its tests pass.

## Role: integrate

Merge the listed piece branches onto your branch in order, fix conflicts,
run the whole test suite, open one draft PR.

### Acceptance
- All piece branches are merged; the whole suite passes; the PR lists each piece.
```

**up.**

```
$ hev swarm up -f logfix.md --dry-run
swarm logfix: owner build-1, standby build-2, 2 roles, 6 units, max 3 concurrent
  orchestrator o-logfix  authority say,notify,rework,retry,dispatch,stop
  migrate    5 pieces  replicas 3  candidates build-1 build-2 build-3 (spread)
  integrate  1 unit    after migrate  pinned build-1
  state ref  hev-swarm/logfix on app's origin
$ hev swarm up -f logfix.md
```

`up` applies `o-logfix` on `build-1`, registers it scope-only on `build-2`
and `build-3`, and as standby on `build-2`. It creates the state ref with
epoch 1 and starts the supervisor. The first tick writes three `attempting`
records and starts `logfix-migrate-api-g1` on build-1, `-worker-g1` on
build-2 and `-cli-g1` on build-3. Each becomes `live` when it appears in
inventory.

```
$ hev swarm ps logfix
ROLE       UNIT     GEN  HOST     STATE    HEAD     RESTARTS
migrate    api      1    build-1  live     -        0
migrate    worker   1    build-2  live     -        0
migrate    cli      1    build-3  live     -        0
migrate    storage  -    -        pending  -        0
migrate    web      -    -        pending  -        0
integrate  -        -    -        waiting  -        0   (after migrate)
```

**A worker dies.** The codex process for `cli` crashes. Its supervisor
records `failed`, exit 1. On the next tick the tick runs `stop` on build-3,
which returns a terminal state, then waits for the 2m backoff. Then it runs
`retry logfix-migrate-cli-g1` in place, with a note giving the exit code and
the last status line. The worktree and the unpushed commit survive. The unit
shows `RESTARTS 1`, generation 1. No model was called. Meanwhile `api`
finishes `done` with a pushed head, and the tick starts `storage` on build-1.

**A host drops.** build-2 stops answering. The `worker` unit shows `live
(host unreachable 4m)`. At 10m the tick records the unit `lost`, fences
generation 1 in `swarm.json` and on the state ref, and computes the base
`d/logfix-migrate-worker-g1` at its last pushed head `9ab2`. It writes
`attempting` for generation 2 and starts `logfix-migrate-worker-g2` on
build-3 with a `## Predecessor` section. build-2 was also the standby. The
owner is still healthy, so the lease is unaffected, and `reply` gets one
notice: "logfix: build-2 lost; worker moved to build-3 (gen 2); standby
unavailable".

Forty minutes later build-2 returns, and `logfix-migrate-worker-g1` is still
running there. The tick sees a fenced run `live`, `stop`s it and marks it
`superseded`. Its branch has two commits past `9ab2`, so the next round gets
`orphaned-work: logfix-migrate-worker-g1, build-2, 2 commits 9ab2..c771`. The
round reads them and `say`s generation 2: "cherry-pick c771 if it isn't
already covered". The fenced worktree stays on build-2 for inspection.

**Integration.** When all five pieces are `done`, the tick starts
`logfix-integrate-g1` on build-1 with the five current-generation branches.
It opens one draft PR. The next round checks the swarm acceptance against the
evidence and writes a reply ("merge-ready, head 51e0, CI green") with
`goal-met.json`.

**down.**

```
$ hev swarm down logfix
stopped o-logfix (goal met)
runs: 0 stopped (none live)
kept: 7 branches (5 pieces, 1 superseded, 1 integration), 1 draft PR
kept: worktree build-2:~/dispatch/logfix-migrate-worker-g1/wt (superseded)
state ref hev-swarm/logfix retained (delete with git push origin :hev-swarm/logfix)
```

A person reviews and merges the PR.

## Borrowed and refused: Docker Swarm and Kubernetes

Borrow:

- **Desired state and a reconcile loop**: Deployments and Swarm services.
  Kit's version is per swarm and code-only.
- **Restart policy with backoff and a cap**: Swarm's `restart_policy
  {delay, max_attempts, window}`, Kubernetes' crash-loop backoff. A capped
  limit, not "always".
- **Placement constraints and spread**: Swarm's `placement.constraints` and
  `max_replicas_per_node`.
- **Stable identity with at most one owner per identity**: a StatefulSet
  does not recreate a pod whose node is merely unreachable until it is known
  to be gone. Kit takes that caution and replaces "known gone" with fencing:
  a new generation on a new branch.
- **Leases with holder identity and renew time** for one active controller:
  Kubernetes `Lease` objects. Here they live on a git ref.
- **`generation` / observed state**, shown by `ps`.

Refuse:

- **A manager quorum (Raft/etcd) or API server.** Kit has no central control
  plane, and origin's compare-and-swap is enough for one lease per swarm.
- **`restart: always`** and replacing work on node loss without fencing.
- **Rescheduling running work away from a full host.** Hosts own capacity.
  Kit queues or waits.
- **Health probes over a network port.** Workers expose no ports. Liveness is
  the supervisor's record, the log, status and pushed commits.
- **Rolling updates and images.** Editing a swarm file changes desired state
  for future launches. Running workers get `amend` or a message, never a
  silent restart.

## Amendments to existing RFCs and docs

| Statement | Where | This RFC |
|---|---|---|
| Authority is `say`, `notify`, `rework`, `retry`, `validate`, `dispatch` | RFC 0008 spec; `orchestrator_spec.go` | **Amends**: adds scoped `stop` |
| The supervisor "runs no model or polling shell loop" | RFC 0008 Phase 2 contracts | **Amends** for swarms: a code-only reconcile tick polls dispatch inventory every `reconcile_every` |
| "Restart/reboot recovery remains outside this change" | RFC 0008 Owner communication contract | **Amends**: swarms add supervisor restart and standby takeover |
| "apply/start must run on the intended supervisor host"; `host` is a placement label | `docs/orchestrators.md`; RFC 0008 Placement | **Amends**: a standby registration permits a supervisor to start there on lease expiry. `host` stays a label. |
| "Owner unavailability fails admission" (scope-only targets) | RFC 0008 Phase 1 | **Amends** for swarms: a target accepts any member of the registered owner set with a current epoch |
| "never start a second worker on a guess"; inspect before retry | `docs/dispatch.md` (Amend, and lost connections) | **Amends** for `effects: branch` swarm roles only: replacement after confirmed stop, or after `host_grace` with fencing. Unchanged for every other run. |
| "A scheduler or queue. Dispatch admission stays the only place capacity is decided." | RFC 0008 Non-goals | **Kept**: the tick proposes a host and admission decides. It is a placement preference, not a scheduler. |
| "No central scheduler, network listener, or additional daemon" | RFC 0012 | **Kept**: the reconciler runs in an existing supervisor, and the hevd watchdog is one call in an existing scan |
| Orchestrators and forwarded runs fail closed for bound senders | RFC 0011 | **Kept**, so swarms do not place on hosts where the owner is a bound sender |
| No merge authority; an orchestrator never writes code | RFC 0008 Non-goals, Open questions | **Kept**: integration is a worker role and a person merges |
| Depth capped at 2; no cycles | RFC 0008 Default off and stacking | **Kept**: a swarm counts as one orchestrator, and swarms do not nest |
| Fixed worker prompt requests a draft PR | `docs/dispatch.md` | **Not amended**: compiled orders override it with an explicit instruction, which dispatch already honours |
| "Hosts own admission and capacity" | `MISSION.md` | **Kept** |

## Build order

Each phase is one order. Phase 1 needs one host only.

1. **Single-host swarm.** `kind: swarm` decoding (`schemas/swarm.schema.json`),
   compilation to an orchestrator spec plus flat orders, `swarm=NAME`
   selector, `stop` authority, `swarm.json` unit table with `attempting`
   records, and a reconcile tick in the supervisor. The tick covers
   partition with static pieces, race with `choose`, `single`/`after`, repair
   1 (in place) with restart limit and backoff, timeout and `stall`. Commands
   `up` (incl. `--dry-run`), `ps`, `logs`, `say`, `pause`, `resume`, `down`.
   All placement is the local host.
2. **Bounds and scale.** `scale` (incl. `--force`), `max_runs`, swarm restart
   limit, `round_failures_limit` hold, plan hold/shrink from
   `hev subscriptions`, the board findings thread, and `pieces: orchestrator`.
   Also from phase 1's list: telling or reworking a running integrate unit
   when a piece changes, a reconcile tick that runs during a long round, and
   nesting a swarm under a plain orchestrator.
3. **Cross-host placement.** Candidates, constraints, spread, admission
   memory, FIFO fallback with queued-unit moves, scope-only registration on
   targets with owner sets and epochs, host-loss detection, repair 2 with
   generations and fencing, stopping returning fenced runs, `orphaned-work`
   evidence. Still one owner and no takeover.
4. **Durable owner.** State ref with compare-and-swap lease, the
   act-only-while-leased rule, supervisor watchdog in hevd's scan and
   `hev swarm sweep`, standby registration and takeover, epoch announcement.
5. **Windows parity and docs.** Detached-supervisor watchdog and sweep on
   Windows, user docs in `docs/swarms.md`, and an end-to-end smoke on three
   isolated local dispatch roots (`config_path`) that kills a worker, drops a
   host and kills the owner.

## Phase 1 as built

Phase 1 follows this RFC with these amendments and clarifications. Each was
found while building it.

- **Attempting records** are re-sent on one host instead of waiting 15
  minutes for `needs-reconcile` (section 5, above).
- **`choice.json`.** An orchestrator's race choice is a file the round writes,
  so code can record exactly one choice. A second, different choice is
  refused. `say NAME choose ROLE/vN` works only for `choose: person`, and a
  round's `choice.json` only for `choose: orchestrator`.
- **`first-green`** chooses the first variant that ends `done` with a pushed
  head and, when the role sets `validation: browser`, a passing verdict. CI
  status is not read yet.
- **Race size.** A race has one unit per variant, or `replicas` identical
  units when it lists no variants. `replicas` is then the concurrency limit,
  defaulting to every variant.
- **`effects: pr`** is allowed only on a `single` role, and at most one role
  per swarm has it. That keeps "one draft PR per swarm" checkable at `up`.
- **Unit state `failed`** means the run's failure was observed, its stop was
  confirmed, and a retry in place is waiting out the backoff. Such a unit
  keeps its replica slot, so a crash does not let another piece jump ahead.
- **A stall** is no change in the run's log size, `status.md` or worktree
  `HEAD` for `stall`. A local commit counts as progress even before it is
  pushed.
- **The tick runs between rounds,** not during one. A round can therefore
  delay repairs by up to `round_timeout`. Running the tick beside a round is
  left for phase 2.
- **`down` stops runs as the operator.** It stops the orchestrator first, as
  this RFC says. After that, admission refuses the swarm's own identity
  because the orchestrator is stopped. `down` stops only the runs in the unit
  table, and units it stopped are recorded `held` with reason `down`. A later
  `up` starts a new lifetime, and its first tick observes those runs again.
- **Required fields.** `reply`, `max_lifetime` and a positive
  `budget.concurrent_runs` are required. `every` defaults to 30m. The
  compiled orchestrator is `mode: loop`, `until: goal`, `idle: skip`.
- **Later-phase fields** (`standby`, `state_repo`, `host_grace`, `plans`,
  `max_runs`, `restart.swarm_limit`, `pieces: orchestrator`, and `placement`
  other than this host and OS) decode, and the schema lists them. `up`
  refuses them with the phase that adds them.
- **Not built in phase 1:** telling or reworking the integrate run when a
  piece changes after integration started; a plain orchestrator managing a
  swarm (RFC 0008's subset rule requires identical selectors, which a
  names-scoped parent cannot meet); the board findings thread. These move to
  phase 2.
- **`ps`** shows the unit table as of the last tick, with `HOST` and `NOTE`
  columns. Without a name it lists the swarms on this host. `say NAME ROLE MSG`
  reaches a `single` role's run without a unit name.

## Open questions

These are for the operator to decide. Each has the default this RFC proposes.

- **Command name.** `hev swarm` or `hev orchestrate up`? Default:
  `hev swarm`. It is the concept the operator asked for, and `orchestrate`
  keeps its single-orchestrator meaning.
- **Where the lease and checkpoint live.** On the work repo's origin, a
  separate private repo, or host-local only (no takeover)? Default: work
  repo's origin with `state_repo` override. Phases 1–3 are host-local
  regardless.
- **Cross-host replacement on by default?** Default: on, for `effects:
  branch` roles, with `host_grace: 10m`. A setting `heal: in-place` limits a
  swarm to repair 1 for operators who want every cross-host move asked first.
- **Race winner.** Default: `choose: orchestrator`. It is reversible because
  a person merges. `first-green` for cheap races.
- **One PR per swarm.** Default: yes, via an `integrate` role. Alternative:
  one draft PR per piece, which gives more review surface and more noise.
- **Answering worker decisions.** Default: `needs-decision` goes to `reply`.
  An opt-in `decisions: orchestrator` would let rounds use `say --resolve`
  when the brief covers the question.
- **Ceilings.** Default: 8 replicas per role, 32 units per swarm. Raise them
  only after phase 3 has run on real work.
- **Bound senders.** Default: excluded. Revisit with per-grant authority under
  RFC 0011.
- **Merge authority.** Default: none, unchanged from RFC 0008.
