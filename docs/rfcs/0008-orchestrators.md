# RFC 0008: Orchestrators — optional, declarative agents that keep a goal moving

> Proposal only: `hev orchestrate` and the orchestrator spec below are not
> implemented. Today's nearest pieces are repeating runs in `hev dispatch` and
> the bounded `hev expedite` loop.

Status: Draft (2026-11-04).

## Summary

An orchestrator is an optional agent that keeps a goal moving by managing
dispatch runs. It doesn't write code, and it acts only through `hev dispatch`
verbs. It is declared in a file, off by default, and runs in rounds: each round
is a fresh agent session that reads a checkpoint, acts, writes the next
checkpoint and exits. The checkpoint, not the session's context, is what lets
an orchestrator live a long time.

"Orchestrator" is the general name for what `hev expedite` does for one
session's runs. `hev expedite` becomes a preset of it, and an event-driven
intake (a tracker poller that turns tickets into orders) becomes an
orchestrator whose events come from a source plugin.

## Motivation

- **Follow-through is the expensive part.** Dispatch starts work well; what
  stalls is the review, rework, retry and validation after it. People end up
  doing that by hand, or run one-off loop scripts that nothing else can see.
- **Long-lived agents rot.** A single session that runs for days grows its
  context until it compacts away the facts it needed. A loop needs an explicit
  compaction or handoff strategy, not a hope.
- **Ad-hoc loops are invisible.** A bespoke expediter or intake poller doesn't
  show up as a running agent, can't be messaged through peers, and has its own
  caps, pause switch and state layout.
- **Everything needed already exists.** Repeating runs, an evidence collector,
  remote control of runs and admission are in kit. What's missing is the
  declaration, the round runner and the visibility.

## Decision

Add `hev orchestrate` and an orchestrator spec. An orchestrator is a supervised
series of rounds on top of dispatch's repeat supervisor; each round is gather
(code) → act (model, scoped verbs) → checkpoint (required). Orchestrators are
off by default, visible like any running agent, and scoped by their spec.

## Goals

- Declarative: one file says what the orchestrator manages, what it may do,
  how often, until when, and with what budget.
- Default off: no service, no orchestrator, no extra process on a host that
  hasn't applied one.
- Ad hoc: start one from the command line with a goal and a run selector.
- Long-lived: unbounded rounds without context growth; explicit rotation and
  handoff.
- Visible: counts as a running agent in peers, tmux, the picker and shell
  prompts; reachable by message.
- Placement-independent: may run on a different host from its runs; the
  default is the same host.

## Non-goals

- Writing code. An orchestrator dispatches workers; it is never one.
- Merging by default (see Open questions).
- A scheduler or queue. Dispatch admission stays the only place capacity is
  decided.
- Encouraging stacks. Nesting is possible, guarded and discouraged.

## Vocabulary

- **Orchestrator**: a named, declared loop that manages runs toward a goal.
- **Round**: one wake → gather → act → checkpoint → sleep cycle.
- **Packet**: the code-built input to a round (spec, checkpoint, inbox, evidence).
- **Checkpoint**: the required, human-readable state a round leaves for the next.
- **Lineage**: the chain of checkpoints since the last rotation.
- **Source**: a plugin that emits events (tickets, issues) into rounds.
- **Authority**: the dispatch verbs an orchestrator may call.

## What kit already has

| Piece | Where | Reused as |
|---|---|---|
| Repeating runs (`every`, `until`, `max_lifetime`) | `internal/dispatch/order.go`, `repeat.go` (supervisor, no overlapping rounds) | The round scheduler |
| Bounded follow-through for a session's runs | `hev expedite`, `internal/expedite` (Plan, Evidence, Round, Driver) | The ad-hoc preset and the evidence collector |
| Remote control of runs | `internal/dispatch/rpc.go`; `say`, `rework`, `validate` | The only verbs an orchestrator may call |
| Admission | `internal/dispatch/admission.go` | Budget, authority and nesting checks |
| Peers, inbox, hevd | `internal/peers` | Visibility and talking to it |
| Plugins | `hev <plugin>` discovery | Source plugins |

## Proposed surface

### The spec

A markdown file: the frontmatter is the spec, the body is the standing brief
(priorities, what "done" means, who to ask, what never to do).

```yaml
kind: orchestrator
name: release-train
goal: Every PR labelled release-0.9 is merged with green CI by Friday.
host: build-1              # where rounds run; default = host of the managed runs
mode: loop                 # once | loop
every: 20m
until: goal                # goal | 2026-11-08 | never (needs max_lifetime)
max_lifetime: 7d
harness: claude
model: claude-opus-5-5
reply: my-session          # peers address that hears about decisions and blockers

manages:                   # scope; nothing outside it can be touched
  runs: {selector: "issue=REL-*"}   # or names: [...] or "mine"
  repos: [app]

authority:                 # default is say + notify only
  - say
  - rework
  - retry
  - validate
  - dispatch               # start new runs within scope

budget:
  concurrent_runs: 3
  runs_per_day: 20
  usd_per_day: 40

handoff:
  rotate_after: 30         # rounds; then a summarised checkpoint starts a new lineage

idle: skip                 # skip | run — whether a round with no new evidence calls the model
source: none               # or a plugin, see "Sources"
```

Specs are applied from any path with `-f`; `hev orchestrate ls` reads applied
specs from `~/.config/hev/orchestrators/`, where `apply` keeps a copy.

### Rounds, checkpoints and handoff

1. **Wake** on the `every` tick, on a source event, or on a peers message. A
   message wakes it early; it never interrupts a round in progress.
2. **Gather** (code, not model): spec, `checkpoint.md`, inbox, and an evidence
   inventory for everything in scope (run states, PRs, CI, validation
   verdicts). This is `internal/expedite`'s collector generalised. With
   `idle: skip` (the default), a round whose evidence and inbox are unchanged
   since the last checkpoint ends here and costs no model call.
3. **Act** (model): one fresh session gets the packet and may call only its
   authority verbs, through `hev` with `HEV_ORCHESTRATOR=<name>` set, so
   dispatch rejects out-of-scope or unauthorised calls.
4. **Checkpoint**: the round must write `checkpoint.md` before it exits: goal
   status, open threads by run name, what it waits on (a named wait with a
   release condition), next action, and anything for `reply`. A round that
   exits without one counts as failed and the previous checkpoint stands.
5. **Sleep.** State lives in `~/dispatch/orch-<name>/` (`spec.md`,
   `checkpoint.md`, `rounds/NNN/{packet.json,checkpoint.md,trace}`); hevd tags
   traces with `orchestrator=<name>`.

**Compaction is the checkpoint.** Each round starts with an empty context, so
nothing accumulates. `rotate_after` handles slower drift: a rotation round
writes a short summary checkpoint, archives the lineage, and the next round
starts from that summary alone.

**Handoff** moves the goal to someone else:
`hev orchestrate handoff NAME --to SESSION` (a person's session picks it up) or
`--to-orchestrator OTHER` (merged into another orchestrator's scope). Handoff is
the last round: it writes a final checkpoint addressed to the new owner and
stops.

### Ad hoc

```
hev orchestrate start --goal "get my open runs merged" --runs mine          # once
hev orchestrate start --goal "keep 0.9 green" --runs "issue=REL-*" --every 30m --until 2026-11-08
```

`start` writes a spec to `~/dispatch/orch-<name>/spec.md` and applies it.
`hev expedite` becomes this preset: `--runs mine`, authority `say` and `rework`,
`mode: once`.

### Sources

A tracker intake is an orchestrator whose events come from outside and whose
act step is code:

```yaml
kind: orchestrator
name: tracker-intake
mode: loop
every: 15m
agent: off                 # deterministic: rules render orders; no model round
source:
  plugin: linear           # any `hev <plugin>` that speaks the source contract
  team: ENG
  rules:
    - {status: Triage, action: triage, template: triage.md, mode: live}
    - {status: Ready,  action: fix,    template: fix.md,    mode: dry-run}
budget: {runs_per_day: 20, concurrent_runs: 1}
pause_file: state/PAUSED
```

With `agent: on`, the same events feed a model round ("is this ticket ready to
fix?") and the rules stay as guardrails. The source contract: the plugin
answers `hev <plugin> --hev-describe` with `{"source": true}`, and
`hev <plugin> events --since CURSOR` prints JSON lines `{id, kind, url, fields}`
plus a new cursor; optional `ack ID`. Caps, pause file, dry-run and
idempotency markers move from the plugin into the orchestrator.

### Placement

Rounds act through dispatch RPC, so an orchestrator can run on any host that
reaches its runs. The default `host` is the host of the runs in scope. `apply`
warns when they differ ("orchestrator on A manages runs on B; evidence reads
cross the network every round"). Same-host keeps evidence local and keeps
rounds alive when a laptop sleeps.

### Visibility

- Each round runs in tmux session `o-<name>` (beside dispatch's `d-<name>`).
  Between rounds a resting placeholder shows state and next wake, so the
  orchestrator stays listed.
- Peers role `orchestrator`, title = goal. `peers send o-<name> "..."` lands in
  its inbox and wakes it early.
- Running-agent counts (picker, shell prompt) include orchestrators; the picker
  gets an Orchestrators group (state, goal, next wake, runs in scope).
- `hev dispatch ls` gains KIND (`run`/`orch`) and ORCH (managing orchestrator)
  columns.

### Default off and stacking

- Nothing runs unless a spec is applied or `start` is called. No daemon, no
  service; a host without orchestrators runs nothing extra.
- An orchestrator's scope can't include another orchestrator unless its spec
  sets `allow_nested: true`. Depth is capped at 2, `apply` warns, and admission
  rejects cycles.

### CLI

```
hev orchestrate apply -f FILE [--dry-run]
hev orchestrate start --goal TEXT --runs SEL [--every D --until X] [--host H]
hev orchestrate ls                      # name, host, mode, state, goal, next wake, runs, spend
hev orchestrate show NAME               # spec, checkpoint, recent rounds, budget
hev orchestrate logs NAME [--round N]
hev orchestrate wake|pause|resume|stop NAME
hev orchestrate handoff NAME --to SESSION | --to-orchestrator OTHER
```

## Build order

1. Spec, `schemas/orchestrator.schema.json`, validator, `apply --dry-run`.
2. Round runner on `dispatch/repeat.go`: gather packet → idle check → one
   session → required checkpoint.
3. `HEV_ORCHESTRATOR` scoping: authority and scope checks in dispatch admission.
4. Checkpoint store, rotation and handoff.
5. Visibility: peers role, `o-` sessions, `dispatch ls` columns, a
   machine-readable `hev orchestrate ls --json` for pickers and prompts.
6. Budget, pause and nesting guards.
7. `hev expedite` as a preset.
8. Source plugin contract and the deterministic `agent: off` path.

## Open questions

Each has a default this RFC proposes; reviewers can overturn it.

- **Merge authority.** Default: there is no `merge` verb in v1. Where policy is
  "workers stop at commit, a person merges", an orchestrator reports
  merge-ready to `reply` instead. Revisit with per-goal merge once rounds have
  a track record.
- **Fresh vs resumed sessions.** Default: always fresh. Resuming saves warm-up
  tokens for rounds minutes apart but reintroduces the context growth this
  design avoids.
- **Talking to it.** Default: peers inbox, read at the next round (woken early).
  An interactive `hev orchestrate attach` is deferred.
- **Idle rounds.** Default: `idle: skip`, so a quiet loop costs nothing but the
  code gather.
- **Where specs live.** Default: anywhere via `-f`, with applied copies in
  `~/.config/hev/orchestrators/`. Repo-local specs are just files passed to `-f`.
