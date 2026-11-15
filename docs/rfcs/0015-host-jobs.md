# RFC 0015: Host jobs — scheduled dispatch with cache-only secrets

Status: Draft (2026-11-13). Design only; no runtime or host changes.

## Scope decision from the owner

The owner narrowed the order after dispatch: only the promo loop is worth saving
among scheduled agent tasks. Retire the other business cadences rather than build
a general agent scheduling platform. Phase 1 carries exactly one recurring agent
order that drafts and schedules social posts, plus plain interval secret callers.
Calendar behavior below settles the design boundary, not a commitment to build it.

## Summary and motivation

Add one new object, a **job**: an owner-applied, host-local schedule for a
finite dispatch execution. Its initial target is an argv command (the earlier Jobs draft's `dispatch exec`).
The promo loop uses the already implemented recurring dispatch order (`every:`,
`until:`, `max_lifetime:`), not a new scheduled-agent object. hevd evaluates the
schedule; dispatch owns admission, execution, stopping, logs and records.
Platform managers keep hevd alive and keep supervising services. Local files
remain sufficient; neither scheduling nor inspection requires a hosted account.
This serves [MISSION.md](../../MISSION.md): hosts own authority and capacity,
and an operator can inspect what happened across a growing factory.

Today each small timer invents a launcher, credential wrapper, log and failure
path. Consolidating only secret reads leaves that sprawl intact. Consolidating
all process supervision would instead turn kit into an operating system.

## Model and rejected alternatives

| Thing | Recommendation | Alternative rejected and reason |
| --- | --- | --- |
| Scheduled command | New job wraps a dispatch exec run | Separate command executor would duplicate admission, stop and logs |
| Promo agent loop | Existing recurring dispatch order with `every:` | New calendar agent scheduler is unnecessary for one independent loop |
| Service | Existing platform unit, inventoried and health-checked by kit | A new service kind duplicates mature restart, boot and dependency supervision |
| Watch | Existing deterministic source/clauses object (RFC 0012-watch) | A cron-shaped watch source confuses time with source transitions |
| Goal follow-through | Existing orchestrator (RFC 0008) | Re-dispatching its standing brief loses checkpoint and scoped authority |

A job is a scheduling attachment, not another order format. A firing produces a
run; it does not mutate the template order. The promo loop drafts and schedules posts at a fixed interval through a recurring
order. Existing repeat supervision already launches fresh bounded rounds with no
overlap; only cache-only secret launch and attribution need integration. If the
agent instead manages other runs toward a goal, use an orchestrator in loop mode.
Do not move its drafting into an orchestrator, whose role is managing work. Reviewing, validating and reworking the same runs toward a
goal is an orchestrator loop with `every:`. Do not schedule `orchestrate start`
each morning. Deterministic invoice/timesheet intake stays a watch if it reacts
to source records; an independent bounded batch scan can be an exec job.

## Declaration and authority

One Markdown file per job, YAML frontmatter and an optional explanatory body.
The initial field/default and command contracts are in the shipping-style
[reference](../../site/src/content/docs/job-reference.mdx); the three complete
[examples](../../site/src/content/docs/jobs.mdx) use the same contract.

```yaml
---
kind: job
name: snapshot-sync
host: worker-a
exec:
  argv: [python3, tools/sync.py]
  cwd: /srv/example/maintenance
schedule:
  every: 15m
  catch_up: one
secrets:
  SYNC_TOKEN: secret://sync-token
  BACKUP_KEY: secret://backup-key
timeout: 10m
overlap: skip
retry: {attempts: 0}
reply: operator-session
---
Sync snapshots; do not print environment variables.
```

`exec` is the only initial job target. `order` is a reserved future target,
rejected by phase-1 validation; it is not needed for the promo loop. In the
future design, `exec` and `order` would be mutually exclusive. Order paths and exec cwd paths are
absolute on the receiving host. `apply` snapshots the order and its acceptance
body, base, harness, worktree choice and limits; it never rereads an editable
template at firing time. Relative dependencies must be included in the reviewed
bundle; unbundled template dependencies are refused. Each apply stores a digest,
owner, revision and resolved host; remote apply uses existing authenticated RPC.
Host override flags cannot silently change the declared placement.

Owner only, like host update: apply, replace, pause, resume, run and remove are
receiver-authorized owner actions, not permissions inherited from dispatch access.
A repository cannot auto-apply its job files. Commands execute as the daemon's
unprivileged account with normal host admission and capacity checks. No sudo,
credential or permission escalation is supplied. Shells require explicit argv
such as `[sh, -c, ...]`; kit never interpolates. Approval of a job authorizes its
stated recurring work, not unrelated outward actions by an agent. Order acceptance,
repository constraints and spending caps still apply at every firing.

Reject unknown keys, secret values in `secrets`, provider references outside
`secret://NAME`, unsupported schedules, missing failure recipient, and unlimited
timeouts. Literal nonsecret parameters belong in argv or the order; job files
never carry secret values. This validation cannot discover arbitrary embedded
secrets in scripts or prose; owners must review those too. Prefer explicit owner
apply over automatic repo discovery because a pull must not install persistence.

## Scheduling and recovery

The interval contract is the initial build. Calendar, nonzero retries and
queue-one below are deliberately deferred design decisions; no retained pilot
needs them. They are rejected by the initial validator, not silently ignored.
The shipping docs expose only the interval command contract.

**Choose hevd**, extending its local scan/reconcile cycle. Its jobs scheduler
also runs in `daemon --secrets-only` mode, so a host need not enable capture or
archive ingestion. Jobs are off until applied. Exactly one daemon per account
and host holds the scheduler lock. Do not add a second jobs daemon or generate
one platform timer per job. The former duplicates lifecycle and cache ownership;
the latter fractures cross-platform DST, overlap, history and attribution.
The cost is deliberate: no job starts while kit is down. Platform units can
survive kit outages and remain the right home for critical boot/recovery work.

Platform launchd/systemd/Task Scheduler starts and restarts **only hevd** for
jobs. It does not wake a sleeping machine. On resume/reboot, reconcile durable
cursors before launching anything. Persist nonsecret state atomically in private
host files: applied revision, enabled flag, UTC cursor, occurrence reservations,
run IDs, retries, terminal outcome and notice receipts. State is not in Layer.
A host-wide lock and dispatch's durable reservation serialize scheduling and
manual invocations. No second host takes over an unreachable host's jobs.

Intervals are fixed-rate UTC slots anchored to the first successful apply time;
reapply preserves the anchor unless the schedule changes. Changing a schedule
starts a new anchor/cursor at apply time, without historical replay. Calendar
uses five-field cron (minute hour day-of-month month weekday), numeric fields,
`*`, lists, ranges and steps; day-of-month and weekday, if both restricted, use
OR. No seconds, macros, random or shell expressions. An explicit IANA `timezone`
is required, including `Etc/UTC` when desired. Nonexistent DST local times are
skipped; a repeated local minute fires once at its first occurrence. Occurrence
identity includes job revision and logical slot, with UTC instant and zone data
recorded. Prefer this fixed policy over a per-job DST option matrix.

`catch_up: none` (default) advances past missed slots without execution.
`catch_up: one` coalesces all missed slots to the most recent one at recovery;
record the missed count and range, never replay an unbounded backlog. Due means
a slot crossed since the previous successful scheduler observation. A newly
applied job waits for its first future slot. Clock rollback never repeats an
accepted slot; forward jumps use catch-up. Interval evaluation uses monotonic
time while alive, durable UTC slots across restarts. Persist the chosen calendar
UTC instant so timezone database updates cannot reinterpret accepted history.

Delivery is **at most one automatic dispatch admission per occurrence**, not
exactly-once external effects. Reserve occurrence and run ID before launch;
uncertain admission is reconciled by that ID, never submitted under a new name.
If process ancestry/birth evidence cannot establish an outcome, mark `unknown`,
hold the job and notify for owner reconciliation. Do not replay ambiguous work.
Admission queues retain the occurrence lock; timeout includes queue time. A job
that needs capacity records deferred admission until its deadline, then fails.

`overlap: skip` records a skipped slot while queued, running or retrying.
`overlap: queue-one` keeps only the newest due slot, replacing older pending
slots with recorded coalescing; run it after the active occurrence settles.
These rules include manual runs. No parallel or replace/kill option in v1.
`run` on a busy skip job refuses; on queue-one it returns the pending invocation
ID. `run` while paused refuses. This avoids accidental parallel posting.

Retry attempts count **additional** attempts after the first, default zero;
each has its own run ID under one occurrence, delay and timeout. Retry only
known nonzero exits or known timeout after the process tree is confirmed dead.
No retry for uncertain launch, agent `needs-review`, failed acceptance, authority
refusal, or invalid spec. Secrets unavailability/budget/cooldown defers the same
attempt until available or its deadline, with no tight loop and no bypass.
Cancellation does not retry. Retrying may repeat external effects: opt in only
for idempotent work with an occurrence key. Prefer bounded explicit retries over
infinite daemon restart policies. `HEV_JOB_OCCURRENCE` is supplied for application
deduplication; it is not proof a remote side effect happened.

## Secrets and accounting

Every child receives only requested resolved secrets plus a minimal platform
execution environment (PATH, HOME or USERPROFILE, temporary-directory and required
Windows process variables). Strip provider tokens, cache overrides and inherited
secret variables. Daemon resolves the whole map atomically through the host cache
at launch, then dispatch's supervisor injects it via private local IPC/anonymous
pipe into the child's environment only. No values in argv, files, RPC payloads,
run records, job revisions or logs. Remote apply sends references only; resolution
is on the target. Failed resolution launches no child. Agent supervisors keep
values in memory for the launch and never place them in an order or report.

Use RFC 0009's cache-only policy, TTL/freshness, request coalescing, provider
cooldown and credential boundary. No direct-provider fallback for a job, including
when cache IPC is down. Neutral references resolve using the host's configured
manager. A job cannot ask kit to run a direct provider read or turn off the cache.
Reserve caller identity `job:NAME` in the authenticated daemon scheduling path;
children cannot claim another job by setting an environment variable. Each real
provider operation, including credential/token and quota operations, counts in
the host/provider budget and records attributed job, occurrence, count and outcome.
Shared coalesced reads charge the host once; attribute the triggering job and
record the other beneficiaries without counting another paid request. Background
operations use the daemon identity. Per-job numbers must sum with background
usage to host totals. Persist budget counters across restart; refusal occurs
before a provider call. Values and reference URIs never enter accounting logs.

The host budget is configured by the host owner, not raised by a job. Hits cost
zero real requests; denied requests and cooldowns remain visible. Independent
hosts sharing one provider account still need explicit allocations: this is not
a global quota coordinator. Do not promise that caches make a quota unlimited.
A new per-job TTL option is rejected: reuse host cache policy initially.

This is a cooperative same-user boundary, not a sandbox. An arbitrary executable
with network access or access to the user's credential files can still call a
provider outside kit. Strip bootstrap credentials, forbid documented bypasses,
and audit scripts; stronger isolation needs separate OS identities/network policy
and is outside this proposal. The Jobs docs give a direct-call inventory procedure.

## Surface, records and failure delivery

`hev jobs apply`, `ls`, `show`, `history`, `logs`, `run`, `pause`, `resume`,
`remove`, `import` and `reconcile` are specified in the reference. `ls --all-hosts`
queries configured hosts, marks unavailable hosts with last-seen time, and exits
nonzero for incomplete inventory. Never interpret unreachable as empty. Mutations
require explicit `--host`; apply uses the file's host. Read APIs preserve host
ownership/privacy boundaries and machine-readable `--json` output.

Jobs listing shows enabled/paused/held, target, next due, last outcome and budget
usage. `dispatch ls` still lists executions (`kind: exec|run|orch`), adding JOB
and OCCURRENCE columns and a `--job` filter. Do not put idle schedule rows among
active processes. `orchestrate ls` keeps its goal/round view and links managed
runs, not scheduled independent orders. The dashboard places Jobs beside Runs
and Orchestrators: host freshness, next due, failures and drill-down to occurrence,
run, logs, report, trace and cost. Service inventory is a distinct read-only group.

Reuse the earlier exec draft: argv, explicit shell, repository worktree or source
snapshot or empty work directory, process-tree stop/timeout and process-birth
identity. Each agent occurrence uses dispatch order admission, a unique name,
optional worktree, report, trace, cost and validation; no direct harness launcher.
Exec runs also get bounded result records and artifacts. Records retain job digest,
scheduled/actual times, missed/skip reason, attempt and exit, without secret values.
Schedule records point to dispatch records rather than copying logs. Keep a compact
terminal ledger after scratch cleanup so cleanup cannot permit a duplicate slot.
Default retain logs/scratch 14 days after terminal, records indefinitely; never
automatically clean active, unknown or review-pending work.

Required `reply` names an owner peers inbox target. Reuse the shared durable
notice queue from watch/orchestrator convergence, not a new webhook client.
Phase 1 includes extracting that queue if necessary. A terminal failure, held
unknown, blocked secrets deadline or failed acceptance queues one notice keyed
by job/occurrence/outcome, with host, run ID, sanitized reason and inspection
commands. Retries emit the terminal failure only; recovery emits one linked
recovery receipt. Repeated identical failures aggregate an incident (first/last,
count) rather than one alert each slot. Pending delivery, accepted, failed and
stale are visible; inbox acceptance is not human acknowledgement. An unreachable
recipient retains a local notice with existing bounded backoff; no silent success.
A daemon crash cannot deliver its own alarm: external host monitoring stays.

## Services: observe, do not supervise

Later service inventory reads platform unit name, loaded/enabled/running state,
last exit and bounded health probe result/age. Disabled-on-disk differs from
loaded and desired-enabled; kit must show all three rather than infer intent.
An owner can annotate desired state and health endpoint. It must not restart,
install or remove those units. Health probes are bounded deterministic observation,
reusing watch source guards and transition notices once the health adapter exists.
Do not advertise that adapter as implemented by RFC 0012-watch. Platform services
can use `hev secrets run --cache-only` at start; credential rotation requiring a
restart remains the owner's platform policy. Do not build service dependencies,
rolling deploys, container orchestration or a universal supervisor.

## Estate mapping

All names and paths here and in the examples are fictional.

| Existing category | Destination | Why / what stays outside |
| --- | --- | --- |
| Dashboards, previews, intranet, board server, media tool | Platform services, later kit inventory/health | Long-lived sockets and restart policy belong to platform |
| Capture daemon and secrets endpoint | One platform-supervised hevd | Must boot independently of its scheduler |
| Snapshots, pruning, sync, backups (5m–6h) | Interval exec jobs | Bounded recurring commands, cache-backed credentials |
| Token refresh | Interval exec if refreshing an external system; cache credential refresh stays in resolver | Do not add a job to refresh kit's own provider token |
| Monitors, heartbeat | Interval exec for finite checks; watch for transition-based notices | Critical external host-down monitoring stays independent |
| Social-post promo loop | Existing recurring dispatch order (`every:`) | Preserve only this agent loop; drafts and schedules posts with explicit authority |
| Daily, weekly, monthly, quarterly business cadence | Retire | No remaining need justifies calendar agent scheduling |
| Invoice and timesheet watchers | Retire unused tasks; retain an existing watch only if independently justified | No new adapters, polling or agent scheduling features for them |
| Linux user timers | Exec/order jobs by payload | Linux user services stay systemd units |
| Windows board and trace site tasks | Platform services/tasks with cache-only startup | Do not relaunch a server each timer tick |
| Windows demo agents | Retire unless separately requested as one-off dispatch work | No scheduled demo-agent platform |
| Windows intake poller | Existing watch; platform unit until in-process watch integration | No duplicate watch scheduler |
| Laptop notice pulling and peers beacon | Interval exec if safe while kit is available | Bootstrap/recovery-critical notice transport stays platform-owned |
| Laptop tmux pruning | Interval exec with bounded timeout | One history instead of custom log |
| Plists present but unloaded | Inventory as present, loaded=false, desired=unknown | Never auto-enable on import |
| Existing orchestrator follow-through loops | Existing orchestrators | Keep checkpoints, scopes, budgets and goal lifecycle |

## Migration and updates

1. Inventory platform declarations **and live state**, wrappers, direct provider
   calls and recipients. Import is read-only: `jobs import --platform ... --unit
   ... --host ... --output ...` proposes a local file and a warning report. It
   parses launchd plist, systemd timer+service, or exported Task Scheduler XML;
   never executes a unit or contacts a provider. Import refuses credentials in
   output, asks for reference mappings, and flags environment files/inline values
   for manual review without echoing them. Unsupported trigger, dependency,
   identity, DST or quoting semantics block apply until explicitly rewritten.
2. Review argv, host, cwd, timeout, recipient and semantics. A service imports
   only as an observation recommendation, never an executable job. Import does
   not disable, unload or delete anything; an unloaded unit yields a paused
   proposal and reports its unknown desired state.
3. Apply paused and compare `show` next slots with the old timer. Run one safe
   dry-run payload through dispatch, inspect logs, secrets request counts and
   report. Keep old installed during the comparison; do not run two production
   side effects. True side-by-side execution is allowed only for read-only or
   idempotent outputs with a shared deduplication key; otherwise use a shadow
   destination or disable the old trigger before a single manual invocation.
4. Disable the old trigger using its platform tool, verify no old child is live,
   then resume kit at a recorded cutover time. `catch_up: none` avoids replaying
   already executed work. Keep the old declaration disabled for rollback. On
   rollback pause kit, settle live work, then enable the old trigger. After a
   successful comparison window, explicitly remove the old unit and wrapper.

Prefer a read-only importer and deliberate cutover to an automatic uninstall:
platform identity and effect semantics cannot be guessed safely.

`hev update` preserves in-flight dispatch supervisors and their versioned binary.
Before restarting hevd, stop admitting new occurrences, flush cursors/reservations,
then restart; do not signal active jobs. Startup reconciles them by run ID and
process birth and resumes observation. Agent environments stay in their existing
processes; lost cache entries refill on demand within budget. Version directories
referenced by active runs must be pinned beyond ordinary retention. Unsupported
job schema pauses that job with a notice; never silently drops fields. State
migration must be backward-readable by the previous build before switching,
otherwise refuse the update and explain. Rollback resumes from the same ledger,
not a clean schedule. No platform service restart beyond existing update policy.

## Earlier Jobs and Order status drafts

The earlier unmerged November Jobs draft used numbers now occupied by swarms and
release train. This RFC is 0015, checked against the tree and open PRs. It carries
forward **dispatch exec**, explicit argv/shell, host admission, source/worktree
choices, artifacts/pull, birth-based liveness and durable bounded run records.
It adds the missing scheduling attachment, cache-only launch, occurrence ledger,
failure recipient and cutover semantics. One-off `dispatch exec` remains useful
without a schedule; jobs invoke that same execution path internally.

Change: use `hev dispatch cleanup` for the eventual execution cleanup namespace,
not the old `hev orders cleanup` shim. Cleanup implementation and launcher-shim
retirement are separate work; phase 1 only needs safe bounded retention and durable
occurrence/run records. Do not import the draft's real estate inventories or
host-specific cleanup actions. Do not adopt its proposed Order status graph as
a dependency: keep current dispatch states and add schedule outcomes separately.
The old companion Order status proposal is a separate decision; its core statuses,
tracker mappings and automatic drift notices are neither silently accepted nor
redefined here. No new Layer jobs collection or nativejob fenced-workflow contract.

## Phased build order and non-goals

1. **Small usable slice:** interval-only exec attachment in hevd; reuse dispatch
   repeat/admission/process supervision rather than create another supervisor.
   Cache-only injection and attributed host-budget accounting, durable interval
   reservations, skip overlap, finite timeout, no retries, local history/logs,
   owner apply/pause/remove and the existing durable failure inbox. Move two
   high-frequency credential-using snapshot/sync commands. Keep exactly one
   agent loop: the promo order through existing `every: 24h`, with finite
   `max_lifetime`, report, trace and cost. Its only launch changes are clean
   cache-only secrets and attribution through the same dispatch seam. Manually
   translate these files and disable their old triggers after comparison.
   A watch is unnecessary: no source transition decides when to draft posts.
   An orchestrator adds no value unless the loop manages separate worker runs.
2. Add a read-only importer and cross-host listing only after the pilot retires
   the old wrappers and measured provider calls fall. Exercise restart, sleep,
   ambiguous admission, budget exhaustion and update rollback. The daily promo
   interval is elapsed time, not a local-wall-clock guarantee; the posting API
   schedules the actual post time. No DST implementation is needed for it.
3. Only if an actual retained command requires wall-clock time, implement the
   calendar contract above and its DST tests. Queue-one, retries and service
   inventory also need a demonstrated retained use case. No calendar agent
   scheduling, business-cadence platform or new watcher framework is planned.

Non-goals: DAGs and dependencies, distributed failover or global quota brokerage,
exactly-once remote effects, root execution, secret persistence, arbitrary cron
extensions, agent authorization by schedule alone, service supervision and a new
watch/orchestrator engine. Reject a broad first phase that cannot retire any
actual secret callers until every platform service has been modeled.

## Open questions (not blockers to the recommendation)

- Default incident aggregation window: propose one hour, with first failure
  immediate and recovery immediate; validate with pilot recipients.
- Budget allocation UX across hosts sharing an account: retain explicit host
  allocations initially; a central allocator would require a separate RFC.
- Service inventory annotations: propose host-local owner files; choose storage
  format with the later health adapter, without extending the job schema.
