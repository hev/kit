# Swarms

A swarm is one Markdown file. It declares a problem, an orchestrator and worker
roles with concurrency limits. `hev swarm up` compiles the file into an
ordinary [orchestrator](orchestrators.md) and ordinary dispatch runs.
The orchestrator's supervisor then runs a code-only **reconcile tick**. The
tick launches units, repairs failed ones in place and starts integration when
the pieces are done. The design is [RFC 0013](rfcs/0013-swarms.md).

This page covers phase 1, which is what exists now. **Every unit runs on the
host where `up` runs.** Cross-host placement, takeover and `scale` come in
later phases.

## The file

```markdown
---
kind: swarm
name: logfix
goal: Replace every deprecated logging call in app with the structured logger, tests green, one draft PR.
reply: owner-session
host: build-1
every: 30m
max_lifetime: 2d
harness: claude
model: claude-opus-5-5
budget: {concurrent_runs: 4, runs_per_day: 20}
reconcile_every: 2m
restart: {limit: 3, backoff: 2m, max_backoff: 30m}
roles:
  - name: migrate
    shape: partition
    replicas: 3
    pieces: [api, worker, cli, storage, web]
    stall: 45m
    order: {repo: app, base: main, harness: codex, model: model-a, timeout: 90, class: code}
  - name: integrate
    shape: single
    after: [migrate]
    effects: pr
    order: {repo: app, base: main, harness: claude, model: claude-opus-5-5, timeout: 60}
---
## Problem

What is wrong and what done looks like.

## Acceptance
- The swarm-level acceptance the orchestrator judges.

## Role: migrate

Instructions for every migrate unit.

### Acceptance
- What one piece must meet.

## Role: integrate

Merge the listed piece branches, run the suite, open one draft PR.

### Acceptance
- What integration must meet.
```

The frontmatter is decoded strictly, so a misspelt field is an error.
`schemas/swarm.schema.json` describes it.

- **Orchestrator fields** use the [orchestrator](orchestrators.md) names:
  `goal`, `reply`, `host`, `every` (default 30m), `max_lifetime`, `harness`,
  `model`, `round_timeout`, `notify_route`, `budget`. `reply`, `max_lifetime`
  and a positive `budget.concurrent_runs` are required.
- **`reconcile_every`** sets the tick interval (default 2m). **`restart`** sets
  `limit` (default 3 per unit), `backoff` (default 2m, doubling) and
  `max_backoff` (default 30m).
- **Roles.** `shape` is `partition`, `race` or `single`.
  - A `partition` runs one unit per piece.
  - A `race` runs one unit per entry in `variants` (order keys that override
    the role's `order`). With no `variants` it runs `replicas` identical units.
  - A `single` runs one unit.

  `replicas` caps how many units of a role run at once. The ceiling is 8 per
  role and 32 units per swarm. `after` names roles that must finish first.
  `stall` is a duration with no progress after which a unit is stopped and
  repaired. `effects` is `branch` (the default) or `pr`. Only one `single`
  role can have `pr`; it opens the swarm's one draft PR.
- **`order`** takes order frontmatter keys except `name`, `host`, `reply` and
  the repeat keys, which the swarm sets.

`up` refuses a file that needs a later phase and names the phase:

| Field | Phase |
|---|---|
| `plans`, `max_runs`, `restart.swarm_limit`, `pieces: orchestrator` | 2 |
| `host_grace`, `placement.spread`, `placement.exclude`, `placement.hosts` naming another host, `placement.os` naming another OS | 3 |
| `standby`, `state_repo` | 4 |

## What up compiles

`hev swarm up -f FILE --dry-run` prints a summary, then the compiled
orchestrator spec and every unit's order, each after a `--- orchestrator NAME
---` or `--- order RUN ---` line. It writes no state. The spec is one that
`hev orchestrate apply --dry-run` accepts. It has `manages.runs: {selector:
swarm=NAME}`, the roles' repos, and authority `say, notify, rework, retry,
dispatch, stop`, plus `validate` when a role uses browser validation.

Each order is a flat dispatch order named `NAME-ROLE-PIECE-g1`,
`NAME-ROLE-vN-g1` or `NAME-ROLE-g1`. Its body is:

- the Problem;
- the role's section;
- an Assignment naming the piece or variant and the branch;
- swarm rules;
- the role's acceptance as `## Acceptance`.

`effects: branch` orders tell the worker to push after every commit and not
to open a PR. An `after` role's order lists the finished units' branches and
heads.

## How it runs

`up` applies the orchestrator and writes `swarm.md` and the unit table
`swarm.json` into `~/dispatch/orch-NAME/`. Then it starts the supervisor. The
tick runs first and then every `reconcile_every`, between rounds. It calls
dispatch on this host with the swarm's orchestrator identity. Target admission
therefore checks scope, authority and budget for code just as it does for a
round. Each tick:

1. **Observes** each unit's run in `dispatch ls`. A run that ends `done` with
   its head pushed completes the unit. `done` without a pushed head counts as
   failed.
2. **Repairs in place** a unit whose run ended `failed`, `timeout` or `died`,
   or that stalled. It runs `stop`, then waits for `show` to report a terminal
   state, then waits the backoff, then runs `retry` with a note naming the
   failure. The worktree, branch and generation are kept, and the unit keeps
   its replica slot while it waits.
3. **Holds and tells `reply`** (one notice) when repair is not allowed:
   - the restart limit is reached;
   - the stop cannot be confirmed;
   - an `effects: pr` run ended any way but a clean `failed` before its PR.

   It also holds a run that ends `blocked` or `needs-review`, and tells the
   next round as well as `reply`. A second stall on the same unit waits for a
   round to judge it.
4. **Settles races.**
   - `first-green`: code picks the first variant that is done with a pushed
     head and stops the others.
   - `orchestrator` (the default): the round is told when every variant has
     ended, and records its pick in `choice.json`.
   - `person`: `reply` is told, and the person runs
     `hev swarm say NAME choose ROLE/vN`.

   Exactly one unit is chosen. Losing branches are kept.
5. **Launches** pending units up to `replicas`. An `after` role waits until
   every unit of the roles it names is done, or for a race until its choice
   is done.

Before every launch the tick writes the unit as `attempting`. If the tick
crashes before the launch, the next tick sends the same start again. Run
names are unique per host and a duplicate start is refused, so there is never
a second run.

Rounds keep RFC 0008's contract: they write no code and merge nothing.
Each round packet carries the unit table, and a round writes `goal-met.json`
when the swarm acceptance is met. A person merges the PR.

## Commands

```sh
hev swarm up -f logfix.md --dry-run   # validate and print; no state
hev swarm up -f logfix.md             # apply; rerun with an edited file to change roles in place
hev swarm ps                          # swarms on this host
hev swarm ps logfix [--json]          # units: role, unit, generation, host, state, head, restarts, note
hev swarm logs logfix                 # supervisor and tick log
hev swarm logs logfix --round 3       # one round's trace
hev swarm logs logfix migrate/cli -f  # a unit's run log
hev swarm say logfix "message"        # to the orchestrator's inbox; wakes a round
hev swarm say logfix migrate/cli MSG  # to one unit's run (ROLE alone for a single role)
hev swarm say logfix choose try/v2    # a person's race choice (choose: person)
hev swarm pause logfix                # no rounds, launches or repairs; workers continue
hev swarm resume logfix
hev swarm down logfix                 # stop the orchestrator, then every current run
```

`down` keeps branches, worktrees and records. Its units are recorded `held`
with reason `down`, and `ps` still reads them. A later `up` starts a new
lifetime and keeps unit records and restart counts. A running swarm accepts
`up` with changed roles in place. If its orchestrator fields changed, run
`down` and then `up`.

## Scope and authority

The orchestrator's scope is `swarm=NAME`. It matches only runs whose metadata
records `swarm: NAME`. Admission writes that field from the swarm's identity
when the run starts. Order frontmatter cannot set it, and a caller cannot
claim it. So the swarm can stop, say to, retry or rework only its own runs.
`stop` is the RFC 0013 addition to orchestrator authority.
