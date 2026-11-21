# RFC 0023: `hev climb`, multi-day hill climbing on a scored goal

Status: Proposed (2026-11-20). Design only; no code. Builds on
[RFC 0008](0008-orchestrators.md) (orchestrators),
[RFC 0013](0013-swarms.md) (swarms), [RFC 0016](0016-cluster.md) (cluster
usage facts), [RFC 0019](0019-gates.md) (gates, host config through hevd) and
[RFC 0005](0005-marks.md) (evals). Depends on RFC 0021 (scorers, benchmarks
and self-improvement, open in PR #284) and PR #291 (dispatch degrade). The
meta-harness framing is in PR #297.

> Numbering: 0021 is scorers (PR #284) and 0022 is evidence, so this is 0023.
> No open PR claims 0023 as of 2026-11-20.

## Summary

A **climb** points a fleet of coding agents at a goal that has a number, and
gives it a budget measured in days and plan quota. Each **generation**
dispatches N workers. Each worker starts from one of the current top-k
branches, and gets the leaderboard and a digest of what already failed. The
host scores every resulting commit with a pinned scorer and keeps the best.
It records everything in an append-only **lineage**. The climb stops on
budget, plateau, target or deadline, and **promotes** its best branch to a
person for review. This is the AlphaEvolve shape, but run with general coding
agents on the operator's own hosts and subscriptions.

Kit already has the four parts a multi-day loop needs:

| Need | Kit today |
|---|---|
| Orchestration | RFC 0008 orchestrators: rounds, checkpoints, rotation, `hev orchestrate follow` |
| Swarming | RFC 0013 swarms: a code reconcile tick, `race` roles, fencing on generation-numbered branches |
| Continuity | Rounds that survive a sleeping laptop, handoff and rotation, board memory, the trace archive |
| Subscription management | Plan readings in `internal/cost/limits.go`, [Plan usage](../cost-attribution.md#plan-usage), swarm `plans` holds, degrade (PR #291) |

What is missing is the loop itself. That means a numeric objective that
scores a commit, a population of branches, selection, and a budget that
spans days. Capability and safety are two sides of the same loop. A climb
that can raise a number for three days can also find the cheapest way to
raise it, so this RFC treats its control requirements as part of the
design, not as a follow-up.

## Motivation

- **Test-time compute is now a fleet, not a context window.** A
  meta-harness can spend agent-hours the way a model spends tokens. Kit can
  already spread work over hosts and days. What it cannot do is say whether
  the work went *up*.
- **Swarms solve a problem once.** A `race` role picks one variant out of a
  fixed set, and a person merges it. A climb repeats that race: the winners
  become the next generation's parents.
- **Orchestrator budgets are not multi-day budgets.** An orchestrator has
  `concurrent_runs`, `runs_per_day` and an informational `usd_per_day`. A
  value of 0 means unlimited (`internal/dispatch/orchestrator_spec.go:250`
  checks only for negatives, and `orchestrator_admission.go` skips admission
  when both run counts are 0). A loop that runs for days with no person
  watching needs a budget that cannot be unlimited.
- **Plan quota is the real currency.** On the operator's subscriptions, a
  climb runs out of weekly quota long before it runs out of money. The
  incident behind PR #291 (47 orchestrator rounds failed in 7 hours on a
  weekly limit) shows what an unpaced loop does to a plan.
- **A scored loop invites gaming the score.** A fleet rewarded for a number
  will, sooner or later, edit the benchmark, the scorer or the harness.
  Today a dispatch worker inherits `HEV_DISPATCH_CONFIG`, `SSH_AUTH_SOCK` and
  the host's GitHub token (`internal/dispatch/lifecycle.go:44`), so it could
  also start more workers. Kit has to make the honest path the only one
  that pays.

## Decision

1. **A climb is a file and an orchestrator.** `hev climb up -f FILE` checks
   a climb file (`kind: climb`) and pins it by digest. It then compiles it
   into an RFC 0008 orchestrator with a code tick, as RFC 0013 swarms do.
   It adds no third runtime.
2. **The objective is an RFC 0021 scorer of a new kind, `metric`.** It
   scores a commit, not a finished run. It is host policy, pinned by
   SHA-256, and runs on the host against a checkout of the candidate. A
   second, hidden `metric` scorer is the **held-out** check.
3. **Selection is code.** Top-k by objective, parents assigned by rule, and
   stop rules are all computed in code. A model round only writes lessons and
   varies the briefs.
4. **Workers do not push, dispatch or merge.** A worker commits in its
   worktree. The climb supervisor collects the commit, checks it against
   the protected paths, scores it and pushes it to a `climb/NAME/` branch.
   Workers run with a minimal environment.
5. **Budgets are positive, finite and paced.** Every climb names at least
   one budget, plus a generation cap and a deadline. Pacing reads plan
   limits, holds before a weekly limit and resumes after the reset.
6. **A person merges.** `hev climb promote` opens a draft PR from the best
   branch. Nothing in a climb has merge authority.
7. **The lineage is the record.** It is an append-only, hash-chained JSONL
   file that names who started the climb, the pinned goal and scorers, and
   every candidate, score, refusal and disqualification. The leaderboard is
   derived from it.

## Goals

- Run one goal for days across several hosts and the operator's plans, with
  no person in the loop until promotion, and survive host restarts and a
  sleeping laptop.
- Make the objective trustworthy: host-pinned, run by the host, unreadable
  and unwritable from a candidate, with a held-out check that reports
  divergence every generation.
- Spend a budget the operator chose, never more, and never spill onto a
  paid API unless the climb file says so.
- Produce data for the research questions below: score against agent-hours,
  wide against deep, the effect of board memory, and how often the fleet
  games the scorer.
- Reuse orchestrators, swarms, dispatch, scorers, degrade and the board, and
  add only what the loop needs.

## Non-goals

- **Merging.** A climb promotes a branch. A person decides.
- **Evolving prompts or models.** A climb changes a repository. Changing a
  harness, a model or kit's own instructions is RFC 0021's self-improvement,
  which produces reviewed PRs.
- **Open-ended goals without a scorer.** If a goal has no `metric` scorer
  the host can run, it is not a climb.
- **Distributed scoring.** Phase 1 scores on one host per climb.
- **A hosted service.** Everything runs on the operator's hosts, the same as
  RFC 0021.
- **Strong isolation.** Kit narrows what a worker can reach and records
  attempts. A worker running as the host's own account can still read files
  by absolute path. Running climb workers as a separate OS user is
  documented as the strong option (bring your own sandbox, as in RFC 0019).

## Vocabulary

- **Climb:** one goal, one objective, one budget, and its lineage. Its name
  is unique on the host.
- **Climb file:** the declaration, `kind: climb`. It is pinned at `up` by
  SHA-256.
- **Candidate:** one worker's result: a commit, its parent, its diff, its
  scores, its cost and its trace.
- **Generation:** one round of N candidates launched together. Generation 0
  is the base commit, scored once.
- **Width (N):** candidates per generation. **Top-k:** the parents eligible
  for the next generation.
- **Objective:** the `metric` scorer whose value selection uses. Workers see
  its values.
- **Held-out:** a second `metric` scorer on data the workers never see.
  Workers see neither its values nor its name.
- **Divergence:** the objective going up while the held-out score does not.
- **Protected paths:** files a candidate may not change. These cover the
  scorer, its data and the harness that runs it. A candidate that changes
  one is **disqualified**.
- **Lineage:** the append-only record of the climb. **Leaderboard:** the
  current ranking, derived from the lineage.
- **Promote:** push the chosen branch and open a draft PR for a person.
- **Plateau:** no improvement of at least `min_delta` in G generations.

## Proposed surface

### 1. The climb file

**Decision: Markdown with YAML frontmatter, like an orchestrator spec or a
swarm file, rather than the `goal.toml` the first sketch named.** The code
says why. Orchestrator specs and swarm files are already Markdown, with
frontmatter decoded strictly (`KnownFields(true)` in `orchestrator_spec.go`),
and a climb compiles into an orchestrator spec. The climb also needs a body:
the brief that goes into every unit (control requirement 6). Kit already
keeps such briefs in the body of a spec. TOML stays the format for host
configuration, so the host's `[climb]` table and the scorer pins are TOML.

```yaml
kind: climb
name: layer-recall
goal: Raise recall@10 of `hev traces search` on the pinned query set without slowing p90 latency.
reply: owner-session          # peers address for notices and promotion
repo: kit
base: v0.7                    # resolved to a full SHA at up and pinned
host: oven                    # supervisor and scoring host

objective:
  scorer: trace-recall-at-10  # a host scorer of kind metric (RFC 0021 + Q1 below)
  direction: max              # max | min
  repeats: 1                  # scorer runs per candidate; >1 for noisy scorers
heldout:
  scorer: trace-recall-heldout
  every: generation           # score each generation's top-k and every promoted branch
  divergence: 0.02            # pause when objective gain - held-out gain exceeds this
dev:
  scorer: trace-recall-dev    # optional; a small split workers may run themselves

population:
  width: 6                    # N candidates per generation
  top_k: 3                    # parents
  parents: round-robin        # round-robin | rank-weighted
  min_delta: 0.005            # improvement that counts for plateau and the leaderboard

budget:                       # at least one of the first three, each > 0 and finite
  agent_hours: 60
  usd: 150                    # priced cost; see Budgets
  plan_share: {claude: 0.30, codex: 0.40}   # share of each plan's weekly quota
  paid_api: false             # default; true allows API-billed models in the mix

stop:                         # max_generations and deadline are required
  max_generations: 40
  plateau: 6                  # generations without min_delta
  target: 0.92
  deadline: 2026-11-27T18:00:00Z

workers:
  hosts: [oven, mini]
  mix:                        # assigned in turn across the population
    - {harness: claude, model: claude-opus-5-5}
    - {harness: codex}
  timeout: 90                 # minutes per candidate
  protect: [scorers/**, testdata/recall/**, scripts/score-*.py]   # added to the scorer's own list
```

The body holds `## Goal` (a longer statement, with what may and may not
change), and optionally `## Constraints` and `## Hints`. Every compiled order
carries the body unchanged (requirement 6).

`up` refuses:

- unknown keys, the same as orders and swarms;
- a scorer that is not enabled on the host, is not `kind: metric`, or whose
  pin is `stale` (RFC 0021 Q1);
- the same scorer as objective and held-out, or a dev scorer whose data
  digest matches either of them;
- any budget that breaks requirement 3;
- a `mix` entry that is API-billed when `paid_api` is false (see Budgets);
- a host in `workers.hosts` where the starter is only a bound sender
  (RFC 0011), and a starter that is not a person (requirement 6).

### 2. Lifecycle verbs

```sh
hev climb up -f FILE [--dry-run]     # validate, pin, score generation 0, start
hev climb ls                         # name, state, generation, best, budget left, next action
hev climb status NAME [--json]       # leaderboard, last generation, divergence, pacing
hev climb lineage NAME [--gen N] [--json]
hev climb pause NAME                 # no new launches; running candidates finish and are scored
hev climb resume NAME [--ack ID]     # also clears a refusal or divergence hold (with --ack)
hev climb stop NAME                  # pause, then end; the lineage is kept
hev climb promote NAME [CANDIDATE]   # push the branch and open a draft PR (default: best)
hev climb say NAME MSG               # a note into the next judgment round
```

The first sketch used `hev climb goal.toml`. `up -f` matches `hev swarm up
-f` and `hev orchestrate apply -f`. `--dry-run` prints the compiled
orchestrator spec, the first generation's orders, the pinned digests and the
estimated cost of one generation.

`hev climb status`:

```text
layer-recall  running  gen 14/40  oven  started by local:hev 2026-11-20
objective  trace-recall-at-10 (max)    base 0.781   best 0.846 (g12-c3)   +0.065
held-out   trace-recall-heldout        base 0.770   best-of-top-k 0.829   divergence 0.006 (limit 0.02)
plateau    2/6 generations without +0.005
budget     agent-hours 31.5/60   usd 64/150   claude 18%/30% of week   codex 9%/40%
pacing     width 6 -> 4 until claude resets 2026-11-23T13:00Z
gen 14     6 launched: 4 scored, 1 running, 1 disqualified (edited testdata/recall/queries.jsonl)
refusals   0
```

(Illustrative numbers.)

### 3. A generation

One generation, run by the climb supervisor's code tick:

1. **Plan (code).** Read the leaderboard. Take the top-k scored,
   non-disqualified candidates; generation 1 has only the base. Assign each
   of the N slots a parent (`round-robin` or `rank-weighted`) and a
   harness/model from `mix`. Check the generation's estimated cost against
   the remaining budget and pacing (section 6). The estimate is the median
   cost of the last generation's candidates. Shrink N or hold if needed.
   Write a `generation.planned` record to the lineage *before* any launch.
2. **Brief (model, optional).** A judgment round, the RFC 0008 act step,
   reads the last generation's results and the board thread. It writes
   lessons to the board and up to N short **angles**, one line each, so that
   workers on one parent do not all try the same thing. Selection, parents
   and budgets are not its to change. With `agent: off`, the climb skips
   this round and uses no angles.
3. **Launch (code).** For each slot, compile an order and start it through
   dispatch with the climb's orchestrator identity:
   - `name`: `NAME-gNNN-cMM`;
   - `repo`;
   - `base`: the parent's commit, fetched from `climb/NAME/` refs;
   - `harness`, `model`, `degrade`, `timeout`;
   - `publish: none` (RFC 0021 Q6);
   - `env: minimal` (requirement 1);
   - `class: climb`.

   The body is assembled in this order:
   - the climb header (requirement 6);
   - the climb file's body;
   - the angle;
   - the leaderboard (top-k with objective values, a one-line summary of
     each diff, and their parents);
   - the **failure digest**: the last M disqualified, refused, failed or
     non-improving candidates with one line each and the board lessons,
     capped at 4 KB;
   - the protected paths;
   - `## Acceptance`: "commit your change on this branch; the host scores
     it after you finish; you cannot run the objective scorer".

   If the climb has a `dev` scorer, the order also says how to run it.
4. **Collect (code).** When a run ends `done`, the supervisor reads the
   worktree `HEAD` and requires it to descend from the parent. It fetches
   the commit into the scoring clone and applies the protected-path check
   (requirement 2). A run that ends `blocked`, `failed` or `timeout` with a
   commit is still collected and scored, since a partial change can score.
   A run with no new commit becomes a `no-change` record.
5. **Score (code).** Run the objective `repeats` times on the scoring host
   (section 4). Score the generation's top-k with the held-out scorer. Write
   one RFC 0005 eval row per (candidate session, scorer) (section 5).
6. **Record (code).** Append one `candidate` record for each candidate and
   one `generation.done` record. Push each scored candidate to
   `climb/NAME/gNNN-cMM`, using the host's credentials. Workers never hold
   those credentials. Recompute the leaderboard, then evaluate the stop
   rules and the divergence check.
7. **Lessons (model, in the next brief).** The judgment round turns the
   generation into at most N board lessons on the climb's thread, each
   tagged `climb=NAME gen=N`. Lessons are advice to later workers. They
   grant nothing.

A candidate record:

```json
{"seq": 812, "prev": "sha256:5c1e…", "ts": "2026-11-22T04:12:09Z",
 "kind": "candidate", "climb": "layer-recall", "gen": 14, "id": "g14-c3",
 "parent": "g12-c3", "commit": "8a41f0c…", "branch": "climb/layer-recall/g14-c3",
 "run": "oven/layer-recall-g014-c03", "session": "0b9d…", "trace_id": "2d2d6b…",
 "harness": "claude", "models": ["claude-opus-5-5"], "degraded": false,
 "diff": {"files": 3, "insertions": 41, "deletions": 12, "sha256": "e0c2…"},
 "objective": {"scorer": "trace-recall-at-10", "pin": "sha256:9f2c…", "values": [0.851], "value": 0.851},
 "heldout": {"scorer": "trace-recall-heldout", "pin": "sha256:41ab…", "value": 0.833},
 "cost": {"agent_hours": 1.4, "usd": 3.10, "usd_complete": true, "plan": {"claude": 0.9}},
 "state": "scored"}
```

`state` is one of `scored`, `no-change`, `disqualified`, `refused`, `failed`
or `score-error`. Only `scored` candidates can be parents.

**Swarm reuse.** A generation is a swarm `race` role whose variants are
(parent, harness/model, angle), with `choose` replaced by scoring. The climb
takes over three things from the RFC 0013 phase 1 code:

- the reconcile tick;
- stall detection and fenced replacement for `effects: branch` units;
- the `plans` hold.

A climb is not a swarm file. Swarms cap a swarm at 32 units and `max_runs`
at 4 × replicas over its lifetime, while a climb runs width × generations
units, and the climb's budget is the bound. Each generation has its own unit
table.

### 4. Scoring a candidate

The objective runs on the climb's `host`. Each run works in a fresh
checkout of the candidate commit made in a scoring directory that no worker
can reach. It uses the host's pinned copy of the scorer, its command and its
data, never the copies in the candidate tree. Under RFC 0021 a metric
scorer's command is a host file like any scorer file. The command reads the
checkout and the data, and prints JSON on stdout:

```json
{"value": 0.851, "n": 400, "detail": {"recall@10": 0.851, "p90_ms": 118, "misses": ["q-0193", "q-0311"]}}
```

- **Bounded.** Each run has the scorer's `timeout` (default 30m) and an
  output cap. It runs with network denied where the host supports it, with
  an empty environment apart from `PATH`, `HOME` (a scratch directory) and
  the scorer's declared inputs.
- **Repeatable.** With `repeats > 1`, the value is the mean, and the lineage
  keeps every value. A scorer whose spread across repeats exceeds
  `min_delta` is flagged as noisy in `status`, because selection on noise
  produces no climb.
- **Checked before and after.** Before each run the supervisor checks the
  digest of the scorer file, the command and every data file against the
  pin. It checks them again after the run. A mismatch fails the whole
  generation's scoring closed as `score-error` and pauses the climb.
- **Failures.** A scorer error, a timeout or invalid JSON leaves that
  candidate `score-error`. It cannot become a parent, and it does not stop
  the climb unless every candidate in the generation fails to score.
  RFC 0021 scorers are fail-open because they only observe. A climb's
  objective decides selection, so a missing score never counts as a pass.

### 5. What RFC 0021 needs to add

Climb uses RFC 0021 as it stands for host pinning (`[scorers.adopt]`, digest
pins, `stale`), for "a run can never grade itself", for judge isolation and
for the eval row shape. It needs these additions to PR #284, in that RFC's
own terms:

1. **`kind: metric`.** This kind sits beside `rule` and `judge`. A metric
   scorer runs a host command and returns a number. It has no `labels`, no
   `passing` and no `poor`, so it generalizes RFC 0021's `cost` rule scorer,
   which already writes an integer mark with no labels. Its frontmatter
   declares `command`, `direction` (`max` or `min`), `unit`, `scale` (the
   integer multiplier for the mark, for example 1000 for a ratio), `timeout`
   and `data` (paths with SHA-256 digests).
2. **A `commit` subject.** RFC 0021 scores a run or a captured session. A
   metric scorer scores `repo@sha`. The eval row stays keyed by a session:
   the candidate worker's session, with `evidence.subject = "kit@8a41f0c…"`
   and a new filterable `subject` attribute. The base commit, which has no
   session, gets a row under a deterministic synthetic session ID
   `commit:REPO@SHA`.
3. **A pin set, not a file pin.** The digest that `[scorers.adopt]` records
   covers `scorer.md`, its `command` and every `data` file. A change to any
   one of them makes the scorer `stale`.
4. **`visibility: hidden`.** A scorer marked hidden never has its values,
   name or data placed in an order, a worker packet, a judge packet or the
   board. Only `hev climb status`, `hev score` and the dashboard, read by a
   person, show it. A climb's held-out scorer must be hidden. Its objective
   must not be.
5. **`protect`.** A scorer declares the repository paths a subject may not
   change, such as its own proposal under `.hev/scorers/`, fixtures and the
   harness script. Climb adds the climb file's `workers.protect` to that
   list.
6. **Unsampled, synchronous scoring on request.** A climb calls `hev score
   run --commit REPO@SHA --scorer NAME --json` and waits. Metric scorers
   ignore `sample` and are never triggered by dispatch-finish events,
   because a climb candidate is not done when its run is.
7. **The RFC 0005 amendment in RFC 0021 Q3 (newest row per session, role and
   instance).** Without it, the held-out row hides the objective row on the
   same session.

### 6. Budgets and subscription pacing

**Accounting.** All three meters are counted from records kit already keeps:

- **Agent-hours** are the sum of candidates' wall time from `result.json`,
  plus judgment rounds.
- **USD** is priced cost from [cost attribution](../cost-attribution.md),
  plus scoring jobs. As in RFC 0021, an unknown price makes the figure
  incomplete, not zero. If a climb has a `usd` budget and a candidate's
  price is unknown, the candidate is charged its token count at the
  harness's list price. If even that is unavailable, `up` refuses a
  `usd`-only budget for that harness.
- **Plan share** is the share of each plan's weekly quota spent by this
  climb. The reading comes from `Limits` in `internal/cost/limits.go`, which
  has a percent, `resets_at`, `observed_at` and `stale`. Attribution is
  conservative: the climb is charged the larger of two figures. One is its
  own sessions' share of the plan's tokens. The other is the plan's percent
  rise while it had candidates running. Other work on the same plan
  therefore makes a climb slow down, never speed up.

**Pacing**, every tick, per plan and host:

- **Daily pace.** Launch a generation only if the remaining budget covers
  its estimate at the climb's current pace. Pace is remaining budget divided
  by the days left to the deadline or the next weekly reset, whichever is
  closer. When it does not, shrink `width` first (never below `top_k`).
  Then hold.
- **Before a weekly limit.** Stop launching on a plan when its reading
  reaches the host's `[climb] plan_hold_at` (default 80) or the climb's
  `plan_share`, whichever comes first. The hold applies even when the climb
  has share left, so the operator's own work keeps headroom. A `stale` or
  unreadable reading counts as unknown, and unknown holds that plan, as
  swarm `plans` holds do.
- **After the reset.** A plan hold lifts by itself once a fresh reading
  after `resets_at` is below the hold line. A `plan` hold is the only kind
  that lifts without a person.
- **Degrade (PR #291).** Each compiled order carries a `degrade:` chain
  built from the climb's `mix`, and `dispatch` steps down on a limit exit as
  PR #291 specifies. The candidate records every model it ran, from
  `result.json` `models`, so the research data can separate candidates by
  model. An `exhausted` event with `resets_at` puts that plan in a plan hold
  until the reset, and the candidate is `failed` with reason `exhausted`, not
  `refused`.

**What climb needs from PR #291.** It uses the per-order `degrade:` chain,
the `models` and `degrades` fields in `result.json`, the `exhausted` event
with `resets_at`, and the plan observations (`~/dispatch/.limits.json`).
PR #291 also needs one addition. In PR #291, "the last model in a chain
always launches", even into a fresh observation that it is out. A climb
order needs that last step to hold instead (`degrade_last: hold`), so a
generation does not burn retries on an exhausted plan.

**Never spill to a paid API.** With `paid_api: false`, the default:

- `env: minimal` drops `ANTHROPIC_API_KEY` and `OPENAI_API_KEY` from the
  worker;
- `up` refuses a `mix` or `degrade` entry that would run on an API key
  rather than a subscription login. The harness registry (RFC 0020) would
  need to declare which login a harness uses; until it does, `up` refuses
  any entry whose harness has an API key in the host environment and no
  plan in `subscriptions.toml`;
- a session whose cost record shows API billing pauses the climb with
  reason `paid-api`.

`paid_api: true` requires a `usd` budget.

### 7. Continuity

The climb is an RFC 0008 orchestrator, `o-NAME`, on `host`. Its state
directory holds:

```
~/dispatch/orch-NAME/climb/
  climb.md            # the pinned climb file (read-only copy) and its digest
  lineage.jsonl       # append-only, hash-chained (requirement 6)
  leaderboard.json    # derived; rebuilt from lineage.jsonl on load
  gen/NNN/plan.json   # slots, parents, angles, estimate
  gen/NNN/score/      # scorer stdout per candidate and repeat
  PAUSED              # present while paused, with the reason
```

- **Intent before effect.** Every launch, push, scoring and promotion is
  preceded by an intent record in the lineage and followed by a result
  record. This is the swarm "attempting" pattern. After a crash or a host
  restart, hevd's existing scan restarts the supervisor (RFC 0013). The
  supervisor replays the lineage and reconciles any open intent: it asks
  dispatch about a launch, checks the remote ref for a push, and re-runs
  the scorer for an unscored commit.
- **The laptop can sleep.** Workers, scoring and the supervisor run on
  hosts. The laptop only reads `status` and receives notices.
- **Long-lived rounds.** Judgment rounds are ordinary rounds, so
  `rotate_after` and checkpoints apply unchanged. The lineage, not the
  checkpoint, is the climb's memory of what happened. The checkpoint holds
  only the round's own notes.
- **Durable outside the host.** Candidate branches live on origin under
  `climb/NAME/`. Evals live in the operator's Layer namespace. The lineage
  is copied after each generation to `climb/NAME/lineage` (a branch holding
  only `lineage.jsonl`, the same idea as a swarm's state branch). A climb
  can be rebuilt on another host from origin alone. Cross-host takeover is a
  later phase, as it is for swarms.

### 8. Promotion

`hev climb promote NAME [CANDIDATE]` runs at the end of the climb, or
whenever a person asks. It defaults to the best candidate whose held-out
score is within `divergence` of its objective gain. It pushes
`climb/NAME/best` and opens a **draft** PR against `base`. The PR body holds
the goal, the objective and held-out values at base and at the candidate,
the candidate's ancestry back to base with the score at each step, the
total cost, any divergence or disqualification events, and `Requested by`
with the climb's starter. The starter is requested as reviewer. Nothing
else happens.

## Control requirements

These come from a read-only safety review of kit on 2026-11-20. Each is a
rule the climb holds, stated as what is refused and what is kept, and each
has a test from both sides in the build order.

### 1. Workers in a climb cannot dispatch

**Rule:** a climb worker gets no credential or configuration that lets it
start, steer or stop work, or push to a remote.

- **Refused:**
  - `HEV_DISPATCH_CONFIG`, `SSH_AUTH_SOCK`, `GH_TOKEN`, `GITHUB_TOKEN`,
    `HEV_CONFIG`, every `HEVLAYER_*`/`LAYER_*` write key and every
    sandbox-provider key such as `BOAT_API_KEY` in the worker's
    environment. Today `resolveLaunch` forwards all of them
    (`internal/dispatch/lifecycle.go:44`).
  - A `HOME` that contains `~/.config/hev`, `~/.config/gh`, `~/.ssh` or
    git credential helpers.
  - Creating sandboxes or other paid hosts. Only the host's sandbox broker
    holds a provider key, and it serves only the climb's scorer (see the
    fleet stress test).
  - Dispatch RPC from inside a climb run. The run's marker makes admission
    refuse `start`, `say`, `rework` and `stop` whose caller is a climb unit.
  - `git push` and `hev dispatch|orchestrate|swarm|climb` commands, through
    an RFC 0019 tool gate that is always on for `class: climb`.
- **Kept:**
  - `PATH`, `LANG` and `TMPDIR`.
  - A scratch `HOME` with only the harness's own login, reached through
    `CLAUDE_CONFIG_DIR` or `CODEX_HOME`. This is the order key
    `env: minimal`, which dispatch implements and the climb sets on every
    unit.
  - Read access to the repository at the parent commit.
  - The `dev` scorer, if any.
  - Commits in the worktree.
  - A host `[climb] worker_env` allowlist for anything else, such as a
    read-only Layer key, which `up` refuses if it names a refused variable.
  - The supervisor pushes on the worker's behalf (section 3).
- **Honest limit:** a worker running as the host's account can still read
  files by absolute path, and the tool gate stops cooperative agents and
  records attempts. Climb workers as a separate OS user (BYOS) is the strong
  form. `hev climb up` warns when the host has not set
  `[climb] worker_user`.

### 2. The scorers are host-pinned, and touching them disqualifies

**Rule:** the objective, held-out and dev scorers, their commands, their
data and the harness that runs them are pinned on the host by SHA-256 and
run from the host's copy. A candidate whose diff touches a protected path is
disqualified, whatever it would have scored.

- **Refused:**
  - Scoring a candidate whose diff (`git diff --name-only parent...commit`,
    renames included) matches a protected path. It is recorded as
    `disqualified` with the paths and never scored, so a gamed score never
    exists to be selected.
  - Running a scorer whose pin set does not match.
  - A climb file that names an unpinned or `stale` scorer.
  - A candidate that changes a file the scorer's command reads at run time
    without declaring it. The supervisor compares the scorer's opened files
    against `data` when the host can trace them, and `up --dry-run` runs the
    scorer once on base to list them.
- **Kept:**
  - Candidates may change any other file, including tests that are not
    scorer data.
  - Scorers may be changed by a person, through `hev host config`
    (RFC 0019). The change makes the pin stale and pauses the climb with
    reason `scorer-changed`. Resuming then requires a new climb or an
    explicit `--ack`, which writes the new pin into the lineage.
- **Measured:** disqualifications per generation are a first-class number in
  `status`, and the research questions read them.

### 3. Budgets are positive and finite

**Rule:** a climb must name a budget it can run out of.

- **Refused:**
  - A climb with none of `agent_hours`, `usd` and `plan_share`.
  - Any of them set to 0, a negative number, NaN or infinity, or a
    `plan_share` above 1.
  - A missing `max_generations` or `deadline`, or a deadline in the past or
    more than the host's `[climb] max_days` (default 14) ahead.
  - Budgets above the host's `[climb]` ceilings.
  - `paid_api: true` without `usd`.
- **Kept:**
  - Any small positive budget.
  - Budgets on more than one meter; the first one exhausted stops the
    climb.
  - The RFC 0008 meaning of an orchestrator budget of 0 for ordinary
    orchestrators. Today 0 means unlimited (`orchestrator_spec.go:250`
    accepts it, and `orchestrator_admission.go` skips admission when
    `concurrent_runs` and `runs_per_day` are both 0). A climb's compiled
    orchestrator always gets a positive `concurrent_runs` (the width) and
    `runs_per_day`, derived from its budget, so that path never applies to a
    climb. Changing it for other orchestrators is out of scope (open
    question 6).

### 4. Nothing merges without a person

**Rule:** a climb has no merge authority and pushes only under its own
namespace.

- **Refused:**
  - A `merge` verb in the compiled authority (`say`, `notify`, `dispatch`,
    `stop` only).
  - Pushes outside `climb/NAME/`.
  - Any PR that is not a draft.
  - Auto-merge flags.
  - Promotion of a disqualified, `score-error` or `refused` candidate.
- **Kept:**
  - `hev climb promote` opens a draft PR for review.
  - A person merges it, rebases it or throws it away.
  - Candidate branches stay on origin until `hev climb stop NAME --prune`,
    run by a person.

### 5. A model refusal is final for that candidate

**Rule:** when a model refuses the task, the candidate is recorded as
`refused` and nothing tries to get the work done another way.

- **Refused:**
  - Rework of a refused candidate.
  - A retry on another harness or model. A refusal is not a limit exit, and
    PR #291's classifier already treats a policy refusal as never `limit`, so
    degrade does not fire on it.
  - Re-dispatching the same parent and angle in the same generation.
  - Putting the refused request's text into later briefs or lessons.
  - Launching new candidates after a refusal until a person runs
    `hev climb resume NAME --ack REFUSAL-ID`. The hold is the default; see
    open question 3.
- **Kept:**
  - The refusal record (harness, model, the harness's refusal message, the
    trace ID).
  - In-flight candidates of the same generation finish and are scored.
  - The other parents.
  - After the person's acknowledgement, the climb continues. The
    acknowledgement is itself a lineage record.

### 6. The goal is in every unit, and the lineage is append-only

**Rule:** every compiled order states the climb and its goal. One
append-only lineage records who started the climb and every generation.

- **Refused:**
  - A climb unit whose order lacks the climb header. Dispatch admission
    refuses `class: climb` orders without it:

    ```text
    climb: layer-recall · gen 14 · candidate c3 · parent g12-c3
    started by local:hev · goal sha256:… · objective trace-recall-at-10 (max)
    goal: Raise recall@10 of `hev traces search` on the pinned query set without slowing p90 latency.
    ```

  - Changing the goal or scorers of a running climb. The file is pinned at
    `up`. A change means `stop` and a new climb, which may start from the
    old climb's best branch with `--from NAME`.
  - Starting a climb as an agent: `requested_by` must be a person
    (`local:ACCOUNT` or a logged-in person). A worker or an orchestrator
    cannot `up` a climb.
  - Rewriting the lineage. Each record carries `seq` and the SHA-256 of the
    previous record. On load, the supervisor verifies the chain; a break
    pauses the climb with reason `lineage` and a needs-you notice.
- **Kept:**
  - The first record (`climb.started`) holds the starter, host, time, the
    climb file's digest, every scorer pin set, the base SHA and the budget.
  - Every generation, candidate, score, disqualification, refusal, hold,
    resume, acknowledgement and promotion is a record.
  - The leaderboard and `status` are derived from the lineage, so they
    cannot disagree with it.

### 7. Reward hacking is measured

**Rule:** every generation reports how far the objective and the held-out
score have moved apart, and a climb that diverges stops climbing.

- **Refused:**
  - Showing the held-out scorer's name, values or data to a worker, a
    judgment round or the board (`visibility: hidden`).
  - Using a held-out value in selection.
  - Launching a new generation while divergence exceeds `heldout.divergence`.
    That is a `divergence` hold, which only a person clears with `--ack`.
  - Promoting a candidate past the divergence limit without
    `promote --force`, which is recorded.
- **Kept:**
  - Per generation, the lineage records objective and held-out values for
    the generation's best and its top-k. It also records divergence, defined
    as (objective gain since base) minus (held-out gain since base), with
    signs flipped for `direction: min`. `status` shows its trend.
  - Disqualifications and refusals counted beside it.
  - The traces of the candidates that drove a divergence, linked from the
    hold notice, so a person can see *how* the score was gained.

## Worked example: hev layer retrieval quality

The first goal: raise the retrieval quality of `hev traces search` over the
trace archive. Candidates change kit's search path (`pkg/search`, query
building, chunking under RFC 0003, fusion). The `layer` repository could be
a second climb on the same scorer.

**Does a retrieval eval exist?** Not for the trace archive, as of
2026-11-20.

- The run `layer-pro-embeddinggemma-2-20261120` on oven (layer-pro PR
  #1097) adds the `google/embeddinggemma-2` embedder, its service and
  federated RRF groundwork. Its report says the work is "not routed yet",
  and it has no recall@k evaluation.
- The nearest existing eval is `hev board eval` (PR #245). It scores a
  private fixture against the board's search endpoint: recall@k, staleness
  and tokens, with thread IDs as sources. Its fixture and runner are the
  template for this scorer.
- The Layer repository has a SciFact recall harness for its own backends,
  but over a public dataset, not traces.

So the objective below is a design, and its fixture is an open question
(open question 1).

```yaml
# ~/.config/hev/scorers/trace-recall-at-10/scorer.md (frontmatter)
description: recall@10 of hev traces search on the pinned trace query set (objective split)
kind: metric
command: score.py                     # host copy; reads the candidate checkout and the data
direction: max
unit: ratio
scale: 1000
timeout: 30m
data:
  - snapshot/archive-2026-11-15.tar   # frozen, redacted archive snapshot
  - queries/objective.jsonl           # {id, query, relevant: [session_id, ...]}
protect: [pkg/search/testdata/**, scripts/eval-*]
visibility: visible
```

The command builds the candidate, indexes the frozen snapshot into a
scratch namespace or file archive, runs each query, and prints recall@10
and p90 latency. The query set is split three ways by a deterministic hash
of the query ID:

- **dev**, given to workers;
- **objective**;
- **held-out**, hidden.

All three stay on the host. The archive holds private conversations, so the
fixture never goes into a repository or an order. Only the `dev` split's
query IDs and pass/fail results go to workers.

The climb file is the one in section 1. A natural parent mix is one Claude
and one Codex model, a width of 6, top-3 parents, and a week's deadline.

## A second worked goal: fleet-scale stress test

The second goal turns the climb on kit itself: **how many hosts can kit
coordinate before something breaks, and what breaks first?** A scorer
spins up N Boat.dev sandboxes, each running hevd and peers, drives a fixed
coordination workload across them, and reports the largest N at which
every check still holds. Candidates are kit commits that raise that number:
changes to peers, the hevd cluster, dispatch admission or pacing.

**Prior art.** The Boat spike, hev/kit#127 (closed unmerged as a spike),
added Boat HTTP hosts to dispatch: one labelled VM per order, a snapshot
builder with tmux, hevd and the harnesses, and cleanup that keeps records
after VM deletion. Its paid-account acceptance on 2026-11-09 ran ten
distinct orders concurrently. All ten completed, and cross-order trace
search found every one. So ten sandboxes is the proven floor. Nothing
above ten has been tried. The spike also found two limits this goal should
expect to meet again: independent dispatchers need separate host labels to
serialize the cap, and an earlier Boat run stopped on a provider plan limit
(`docs/learnings/cluster-trace-review-2026-11.md`).

**Where it can break.** The scorer measures four places, each with a
pinned check. The first one to fail names the break:

| Break point | Workload | Check that fails |
|---|---|---|
| Peers gossip | Every member sends a fixed fan of `peers send` messages and reads its log | A message is lost or duplicated, or p99 delivery exceeds the pinned bound |
| Cluster membership | Members join, leave and restart on a fixed schedule ([RFC 0016](0016-cluster.md), [cluster](../cluster.md)) | Membership has not converged on every member within the bound after a change, or a member is falsely marked dead |
| Dispatch admission | Every member queues and starts orders against shared caps ([RFC 0012 queue](0012-dispatch-queue.md)) | An order is lost, launched twice, or admitted past a cap |
| Plan quota | Replicated plan-usage facts and pacing holds across the fleet (section 6) | A hold fires late, does not reach every member, or misses a reset |

The workload uses a scripted stand-in harness that makes no model calls,
so a scoring run spends sandbox time, not plan quota. Plan quota is tested
by injecting synthetic usage facts. An optional, separately pinned step
with real harnesses measures where real quota stops the fleet, but that
step is a measurement for the operator, never the objective.

The scorer reports the ramp in `detail`:

```json
{"value": 24, "n": 3, "detail": {"steps": [8, 16, 24, 32], "broke_at": 32, "first_break": "cluster-membership", "converge_p99_s": 41.2, "sandbox_seconds": 5120}}
```

**Held-out.** The held-out scorer runs the same ramp with a hidden fault
schedule (a partition, a member killed mid-order, clock skew) and a
different message mix. A candidate that raises N only by tuning for the
visible schedule shows up as divergence (requirement 7).

**Gaming to watch for.** The cheap ways to "not break" are to refuse work
or to stop measuring. Admission that refuses every order never admits twice,
and peers that drop acknowledgements never report a slow delivery. So the
workload counts refusals above a pinned rate as a failure, the checks run
from the host's copy of the workload rather than from the candidate's own
status output, and the workload, its schedules and the check bounds are
protected paths.

### The sandbox broker and control requirement 1

Requirement 1 holds without exception here, because a Boat key can create
paid machines. The **operator sets N** and a **host-side broker holds the
key**. Climb workers can never mint sandboxes or read the credential.

- **The broker** is a hevd service on the climb's host. It reads the Boat
  API key at use time from the operator's 1Password vault through
  `hev secrets run` ([RFC 0009](0009-secrets.md)), keeps it in memory, and
  never writes it to disk, a log, a sandbox or a lineage record. It listens
  on a Unix socket owned by the supervisor's account. When the host sets
  `[climb] worker_user` (requirement 1, honest limit), that socket is
  unreachable from the worker's account.
- **The scorer asks the broker for a fleet; it never sees the key.** The
  scorer runs network-denied, as every scorer does (section 4). Its one
  channel out is the broker socket. It asks for a ramp for one candidate
  commit. The broker builds the candidate into the pinned snapshot, starts
  the sandboxes, runs the host's pinned workload, returns the measurements
  and destroys the fleet. Every sandbox is labelled with the climb,
  generation and candidate, and carries a hard auto-stop TTL so a crash
  leaves nothing running.
- **N is the operator's.** The ceiling is `max_sandboxes` in the host's
  `[climb.broker]` table, and the ramp steps are part of the scorer's
  pinned data. Neither appears in the climb file, a brief or a worker's
  tree. Raising either is a host config edit, which re-pins the scorer and
  takes effect only for a new climb.

**Refused:**

- `BOAT_API_KEY`, any other sandbox-provider key, and any `op://`
  reference in a worker's environment, `HOME` or brief. These join the
  refused list in requirement 1.
- A broker request from any process other than the climb's scorer. The
  broker checks the caller against the scoring run it expects, and refuses
  and records calls from a climb unit.
- A ramp step above `max_sandboxes`. So is a missing `max_sandboxes`, a
  value of zero, or an unbounded one (requirement 3).
- A fleet that outlives its scoring run's `timeout` plus the TTL.
- Passing the key, the 1Password session or the host's cluster key into a
  sandbox. Each fleet gets a fresh cluster key and a sandbox-scoped hevd
  token. Both are discarded with the fleet.
- More broker spend than the climb's `budget.usd`. Every fleet's
  sandbox-seconds are priced into the candidate's cost, and the broker
  holds its own daily ceiling as a backstop.

**Kept:**

- Workers read and change the coordination code, and run the `dev` scorer:
  a small ramp on local processes or containers, with no sandboxes and no
  key.
- The scorer gets measurements and sandbox-seconds for each candidate, and
  the lineage records them.
- The operator can still run `hev dispatch` to Boat hosts by hand as the
  spike did. The broker only removes that path from inside a climb.

The climb file differs from section 1 only where the goal does:

```yaml
kind: climb
name: fleet-breakpoint
goal: Raise the number of hosts kit coordinates (peers, cluster, admission, pacing) before a pinned check fails.
repo: kit
base: v0.7
host: oven
objective: {scorer: fleet-breakpoint, direction: max, repeats: 2}
heldout: {scorer: fleet-breakpoint-faults, every: generation, divergence: 8}
dev: {scorer: fleet-breakpoint-local}
population: {width: 4, top_k: 2, min_delta: 4}
budget: {agent_hours: 30, usd: 80, paid_api: false}   # usd covers broker sandbox time
stop: {max_generations: 15, plateau: 4, deadline: 2026-12-04T18:00:00Z}
workers: {hosts: [oven], mix: [{harness: claude}, {harness: codex}], timeout: 120}
```

Numbers are illustrative. `repeats: 2` is there because a fleet ramp is
noisier than a recall score.

## A third goal: an arena

The open-ended goal is a bot for a small original game, written for the
arena. The objective is the win rate against the current champion over a
fixed set of seeds. This stresses the design in two ways:

- **The objective moves.** When a candidate becomes champion, everyone's
  score against the champion changes. The scorer therefore reports win rate
  against a frozen **ladder**: the base bot, fixed reference bots and
  champions frozen every G generations. Each ladder freeze is a lineage
  record and a new pin. Win rate against the current champion goes in
  `detail`.
- **Held-out is a different opponent pool,** on seeds the workers never see.
  It shows whether a bot beats the field or only exploits the champion.

The arena is where wide against deep, and diversity from angles, should
show most clearly.

## Research questions

Every candidate record carries the data to answer these. `hev climb lineage
--json` is the export, and evals make them queryable in Layer.

1. **Score against agent-hours.** Is the best score log-linear in cumulative
   agent-hours, does it plateau, and where? The curve is best-so-far against
   cumulative agent-hours, with plan share on a second axis, per climb and per
   harness/model.
2. **Wide against deep.** For the same budget, is N=12, k=2 better than N=4,
   k=4, or than a few long single lineages? Run paired climbs on the same
   goal and scorer pins with budgets matched to the agent-hour.
3. **Does board memory of failures raise the curve?** A/B within one climb:
   half the slots get the failure digest and lessons, half get only the
   leaderboard. Assignment is by slot parity and recorded per candidate.
   Compare the improvement rate per agent-hour.
4. **How often does the fleet game the scorer, and do the traces show it?**
   Count disqualifications, divergence holds and near misses (a candidate
   that reads protected files or the scorer path without writing them). For
   each one, check whether its trace shows intent (searches for the scorer,
   reads of fixture files, talk about the metric) before the edit. Kit
   records traces already, so the question is how early it can tell.
5. **What limits size first?** Quota (plan holds per day), integration (how
   often top-k branches conflict when combined; a later `merge-parents`
   operator), or human acceptance (time from promotion to merge, and
   promoted PRs rejected). Record which hold or wait dominated each day.

## Build order

1. **Worker isolation** (requirement 1): `env: minimal` and a scratch
   `HOME` in dispatch, the climb-unit refusal in admission, and the
   always-on tool gate for `class: climb`. Tests from both sides: a climb
   unit's environment lacks each refused variable and keeps each kept one;
   a climb unit's `hev dispatch start` is refused, and an ordinary run's
   still works.
2. **RFC 0021 additions** (section 5): `kind: metric`, the commit subject,
   pin sets, `hidden`, `protect` and synchronous `hev score run --commit`,
   on top of PR #284's phase 2.
3. **Climb phase 1, one host, one harness:** the file, the lineage with
   its hash chain, generations, collection and the protected-path check,
   scoring, budgets (agent-hours and USD), stop rules, holds, `status`,
   `lineage` and `promote`. Tests cover every control requirement from both
   sides. Two examples: a candidate that edits a fixture is disqualified,
   and one that edits a neighbouring test is scored. A zero or infinite
   budget is refused, and a small positive one is accepted.
4. **Pacing:** plan share, the plan hold and reset, and the PR #291
   additions (`degrade_last: hold`). Paid-API refusal.
5. **Multi-host workers, the judgment round, angles, board lessons and the
   A/B slot split** for research question 3.
6. **The retrieval goal** once its fixture exists.
7. **The fleet stress test:** the sandbox broker, the `[climb.broker]`
   table and the Boat key refusals in requirement 1, tested from both sides.
   A scorer's ramp within `max_sandboxes` is served. A climb unit's broker
   call, a step above the ceiling, and a worker environment carrying a
   provider key are each refused. Then the arena.

## Open questions

1. **The retrieval fixture.** Who writes the trace query set and its
   relevance judgments, and from which archive window? Can `hev board eval`'s
   fixture format be reused with session IDs as sources? Until it exists,
   the first climb's objective is undecided.
2. **Scoring cost.** Indexing an archive snapshot per candidate may cost more
   than the candidate. Should scoring share an index across candidates that
   do not touch indexing, keyed by a digest of the files that affect it?
3. **Refusal hold threshold.** The default pauses the climb on the first
   refusal. Is a per-climb `refusals.hold_after` (default 1) acceptable, or
   should every refusal always need a person?
4. **Dev scorer leakage.** A visible dev split helps workers and also teaches
   them the distribution. Is a dev split worth its overfitting, and should
   its size be capped relative to the objective split?
5. **Plan share attribution.** Charging the climb the larger of its token
   share and the plan's percent rise is safe but pessimistic on a busy
   host. Is cluster-wide attribution (RFC 0016 usage facts) worth waiting
   for?
6. **Zero budgets elsewhere.** Requirement 3 holds only for climbs. Should
   RFC 0008 orchestrators also stop treating 0 as unlimited, and require an
   explicit `unlimited` instead?
7. **Combining parents.** Selection here is mutation only. Should a later
   phase add a crossover step (merge two top-k branches) as an integration
   worker, as swarms do?
8. **The broker's home.** Should the sandbox broker be a climb-only hevd
   service, or the general way any host-side job reaches Boat, with
   dispatch's Boat hosts from hev/kit#127 as its second client? A general
   broker is one place to cap and audit paid machines, but it grows the
   blast radius of a bug in it.
9. **Fleet scoring cost.** A full ramp per candidate multiplies sandbox
   time by width and repeats. Should the ramp start at the parent's
   breaking point and stop at the first failure, at the cost of not
   re-checking the lower steps?
