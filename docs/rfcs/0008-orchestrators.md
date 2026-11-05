# RFC 0008: Orchestrators — optional, declarative agents that keep a goal moving

> Phase 2 implements visibility, the expedite preset, source plugins and
> deterministic source rules. Picker/shell integration and real tracker plugins
> remain outside kit. See [usage](../orchestrators.md).

Status: Phase 2 implemented (2026-11-04).

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
hev orchestrate say NAME MSG
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

## Phase 1 admission and lifecycle refinements

`round_timeout` bounds each gather/session/checkpoint round and defaults to
`30m`. It accepts the same positive durations as `every`. Expiration kills
the harness process tree, fails the round, and preserves the previous accepted
checkpoint. Accepted rounds archive consumed messages in `rounds/NNN/inbox.md`;
messages appended during a round remain pending.

Apply on the owner to run rounds. On each remote dispatch target, use
`hev orchestrate apply -f FILE --scope-only-owner OWNER`, where OWNER is a
configured remote host pointing back to the owner. This registers the spec
without starting a supervisor. Target admission checks scope and authority
locally and asks the owner to check lifecycle before each operation. Owner
unavailability fails admission. Local target lifecycle files do not decide
whether the owner's orchestrator has completed or paused. Reapply without this
flag to explicitly turn a registration into a supervisor owner.

Nested specs must narrow the parent's authority, repositories and run names.
Selectors must match exactly (including reply identity for `mine`); arbitrary
selector containment is not inferred. Positive child run and informational spend
limits cannot exceed the parent's limits or become unlimited. Child attempts
also consume the parent's budget. Owner budget snapshots consult the selected
target and hosts with known scoped runs from local routing records and prior
round packets. An unavailable relevant host fails admission; unrelated hosts
are not consulted. Runs introduced independently on previously unknown hosts
must first be gathered into the owner's evidence inventory.

Terminal `wake`, `resume` and `handoff` return errors. Reapply deliberately to
begin a new lifetime. Scope-only registrations cannot launch or supervise rounds.

### Trust boundary

`HEV_ORCHESTRATOR` carries identity, not authentication. On Linux, dispatch also
recognizes descendants of a recorded live round harness through process ancestry
and enforces that identity even when the environment variable is cleared. Worker
supervisors clear orchestrator identity, and worker-internal validation result
and reservation RPCs ignore it. On other operating systems identity currently
relies on the environment and request field.

These checks constrain cooperative harnesses; they are not a security sandbox.
A harness with unrestricted access to the owner's account can edit specs or
state, escape its recorded process tree, change HOME, invoke a remote command
outside the RPC client, or forge RPC identity and worker-internal requests.
Owner lifecycle checks and process ancestry do not authenticate remote callers.
An OS sandbox and authenticated capabilities are required to contain hostile
code. Phase 1 must only run trusted harnesses with trusted dispatch endpoints.

## Phase 2 contracts

On Unix the supervisor and each fresh round share tmux session `o-NAME`.
Between rounds the supervisor is the resting placeholder: its pane prints the
state and next wake, then blocks on filesystem notifications and a deadline.
It runs no model or polling shell loop. The session has `@peers-role=orchestrator`
and pane title equal to the goal. Terminal completion, handoff and stop exit the
session. Windows retains the detached supervisor path. The peers CLI and shell
pickers are separate consumers; this phase does not modify them.

`hev orchestrate say NAME MSG` appends a message and a durable wake marker.
`hev dispatch say o-NAME MSG` routes to the same owner operation. A running round
finishes before the next wake; messages appended during a round remain pending.
Paused orchestrators retain messages until resumed. Terminal ones reject them.
`hev dispatch ls` includes `KIND` (`run` or `orch`) and `ORCH` (comma-separated,
name-sorted matching orchestrators). JSON rows have `kind` and `orchestrator`;
orchestrator rows use name `o-NAME`. Scope-only registrations annotate managed
runs but do not create an owner row. Dispatch capacity continues to count workers.

### Stable orchestrator inventory

`hev orchestrate ls --json` always returns an array, including `[]` when none
are applied. Each object has these fields; consumers must ignore added fields:

| Field | Meaning |
| --- | --- |
| `name`, `goal`, `mode` | Applied identity, goal, and `once` or `loop` |
| `host` | Optional placement label; absent means local owner |
| `state` | `ready`, `running`, `waiting`, `idle`, `paused`, `failed`, `stopped`, `complete`, or `handed-off` |
| `start`, `next_wake` | RFC 3339 UTC timestamps; the zero timestamp `0001-01-01T00:00:00Z` means no scheduled wake |
| `rounds`, `lineage` | Persisted round count and rounds in the current checkpoint lineage |
| `managed_runs`, `runs_today` | Last gathered scope size and current UTC-day admitted attempts |
| `spend_usd` | Number when available, otherwise `null` (currently always unknown) |
| `error`, `digest` | Optional latest failure and opaque idle-evidence digest |

Rows are sorted by name. State files may change while inventory is collected;
this is an observational snapshot, not a reservation or liveness guarantee.

### Source wire format and state

Plugins must be installed as ordinary `hev-PLUGIN` executables on PATH. The
owner execs `hev-PLUGIN` directly (found with PATH lookup; `.cmd` batch plugins
run under `cmd.exe /d /s /c`) in its own process group on Unix or a
kill-on-close job object on Windows, with a bounded `WaitDelay`. A round
timeout or stop kills the plugin's whole tree, including grandchildren that
inherited its pipes. Plugin stderr is appended to `supervisor.log`, prefixed
and capped at 64 KiB per call. Configured `source.args` (for example
`[--config, tracker.toml]`) follow the verb on every call.

`hev-PLUGIN --hev-describe` returns a JSON object with `source: true`; optional
`ack: true` enables acks, and `ack_batch: true` selects the batch ack form.
Events are read with `hev-PLUGIN events`; configured `team` adds
`--team TEAM`. The cursor is passed in the `HEV_SOURCE_CURSOR` environment
variable, never argv, so batch plugins never expand it. The initial cursor is an
empty string. Stdout is JSON lines: event objects with
nonempty string `id` and `kind`, string `url`, and object `fields`, followed by
exactly one separate final `{"cursor":"NEW_CURSOR"}` line. No records follow
that line. IDs must be unique within a batch. Each line is limited to 4 MiB;
command failures, invalid JSON or missing cursor fail the round. Cursor values
are opaque strings.

Acks run only for events that produced a receipt, after the accepted
checkpoint and before the cursor is stored. The single form is
`hev-PLUGIN ack --run NAME --mode MODE -- ID` (the `--` keeps IDs that begin
with `-` from parsing as flags). The batch form is `hev-PLUGIN ack --batch`
with one `{"id","run","mode"}` JSON line per event on stdin. `mode` is the
receipt mode (`live`, `dry-run`, `rejected`, `attempting` or
`needs-reconcile`); `run` may be empty for an event rejected before naming.
An ack failure keeps the old cursor, so the replayed batch acks again: **ack
must be idempotent.**

The owner stores `source-cursor.json` only after accepting a checkpoint and any
requested ACKs. Failed and deferred rounds retain the old cursor. `agent: on`
includes the batch in `packet.json.source`; `agent: off` never invokes a
harness. Applied specs default to `agent: on`. With rules configured, model
dispatches must match a live rule's rendered order in the current packet;
models cannot promote dry runs or invent events. For `agent: off`, target
admission accepts only the runner's own rendered artifact in the current round
directory. Existing scope, authority and run budgets still apply. Apply
refuses a `live` rule unless the spec has `dispatch` authority.

Rules match `fields.status` exactly. Each status has at most one rule, with a
valid action name, a template and explicit `live` or `dry-run` mode. Templates
are Go text templates with `.Event` (`ID`, `Kind`, `URL`, `Fields`),
`.Events` (all members; one element for ordinary rules), `.Action` and
`.RunName`. They render complete dispatch orders, including frontmatter and
`## Acceptance`. The name must be `{{.RunName}}`. Rendered repository and issue
must remain in the applied scope. Every printed template value is checked:
a value containing CR, LF or NUL fails with `ErrSourceUnsafeValue`. Values
that are only tested in conditions, or never printed, are unrestricted. The
rendered frontmatter keys must equal the template's own literal frontmatter
keys (`ErrSourceFrontmatter`), so an event cannot add a key such as `host`.
CLI apply resolves relative templates beside the input spec; the lower-level
apply API resolves relative templates in the orchestrator state directory.
`hev orchestrate render NAME --event FILE|-` previews rules for an event
object, array or JSON lines through the live render path. It writes no
receipt, artifact, budget or state.
Example template:

```markdown
---
name: {{.RunName}}
repo: app
issue: {{.Event.ID}}
---
{{.Action}}: {{.Event.URL}}

## Acceptance
- Complete the requested work and report validation.
```

Run names are deterministic. By default the owner derives
`src-ORCH-HASH` from the plugin, event ID, action and mode. A rule with
`name: event` uses the event ID itself when it passes run-name validation and
the spec's `name_pattern` (a regular expression matched against the whole ID).
Otherwise the event gets a `rejected` receipt (`ErrSourceEventName`). Names
beginning `o-` are reserved for orchestrator rows. A rule with `batch: true`
dispatches every matching event without a receipt as one run, named from a hash
of the sorted member IDs, and writes a member receipt per event. A later batch
of the same rule waits for `cooldown` (default `6h`), and an active or still
attempting batch of that rule blocks a changed set. Batch rules cannot use
`name: event`.

`pause_file` is absolute or relative to the applied spec. CLI apply stores it
absolute, resolved beside the input file. The API resolves it beside the stored
`spec.md` in the owner state directory. The supervisor watches the file and
starts a round promptly when it is removed. While it is present, it blocks
source polling, rule actions and model admissions.

Receipts live in `source-markers/`, keyed by plugin, event ID, action and mode,
and store the event, so retries and reconciliation never depend on the plugin
replaying it. The modes are:

- `dry-run`: artifacts and a receipt, no dispatch. Changing to live is a
  distinct receipt.
- `attempting`: written before the budget charge and the live dispatch.
- `live`: the run was admitted. "Run already exists" for the deterministic
  name also counts as admitted.
- `rejected`: a definitive failure, such as a render or parse error, an
  out-of-scope order, an unknown target, or an admission or budget refusal from
  a target that answered. It is retried from the stored event with exponential
  backoff (5 minutes doubling, at most 6 hours), and the batch continues.
- `needs-reconcile`: an `attempting` receipt whose run is still absent after
  the 15-minute grace period.

Only an unconfirmed launch, a transport failure or cancellation is uncertain.
The admission probe runs full target admission. A transport failure during the
probe writes no receipt, because the probe mutates nothing. An uncertain
dispatch keeps `attempting`, halts further live dispatch in that round and
keeps the cursor. Later rounds never re-dispatch it. They promote it to `live`
when the run appears in the target's inventory, and after the grace period
mark it `needs-reconcile`. Inspect the target, then run
`hev orchestrate reconcile NAME --event ID --admitted` (receipt becomes
`live`) or `--not-admitted` (receipt becomes `rejected` and retries on the next
round). Batch members sharing the run are reconciled together.

Deferral is not failure. A run cap, a `pause_file`, a batch cooldown or an
active batch defers live units. Dry-run units behind a cap still process.
State stays `waiting` with `deferred` naming the reason, and the cursor is kept.
A round in which everything was deferred creates no round directory and uses no
round number. A `mode: once` orchestrator with deferred work stays `waiting`
until it is woken. The source runner owns UTC daily counts
(`source-budget.json`) and scoped concurrent caps. Batch state lives in
`source-batches/`.

Intake health signals are edge-triggered, once per receipt and kind:
`attempt-stale` (an `attempting` receipt past grace whose run lookup failed),
`rejected` (a new rejected receipt) and `needs-reconcile`. The last 50 are kept
in `state.json` `signals` and shown by `show` and `ls --json`. Each new signal
is also passed to `dispatch.IntakeSignalHook`, where owner notices attach. This
phase installs no notice transport.
No real tracker plugin is shipped; synthetic fake plugins live under testdata
only.

`hev expedite` launches the common runner with the `mine` scope, `say,rework`
authority and `mode: once`. Its snapshot, flags, status/stop commands and explicit
review-evidence utility remain available. The legacy `--every` and
`--max-lifetime` values are retained in the saved snapshot. A compatibility scheduler reads
that snapshot to repeat the once preset at the requested bounded cadence;
ordinary once orchestrators still exit after one round. Merge-ready findings return to the owner: the preset
has no merge authority. The separately invoked `expedite round` utility retains
its explicit reviewed merge-on-green checks for legacy orders.
