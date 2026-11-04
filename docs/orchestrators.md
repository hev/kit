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
final checkpoint addressed to the new owner and ends the supervisor; it does
not send a peer message in this phase.

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
implemented. See [Phase 2 contracts](rfcs/0008-orchestrators.md#phase-2-contracts)
for the stable JSON inventory, source wire protocol, templates, receipt recovery
and scheduling semantics. No real tracker plugin is included. Picker and shell
integration, the separate peers CLI, and handoff to another orchestrator remain
outside this phase.

Rounds default to a `round_timeout` of `30m`; timeout fails the round and kills
the harness tree. Accepted inbox messages move to `rounds/NNN/inbox.md`.
Nested specs must be subsets and share their parent's run budget. Terminal
wake/resume/handoff calls fail until reapply. See RFC 0008's trust boundary:
these admission checks require trusted harnesses and do not replace sandboxing.
