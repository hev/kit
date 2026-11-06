# Run a follow-through orchestrator

A follow-through orchestrator is a standing, agent-driven loop that takes
dispatched runs from "the worker finished" to "a human can merge this". Each
round reviews new work with a different harness from the implementer's,
sends rework, checks CI itself, keeps named waits in its checkpoint, and lists
merge-ready PRs for a person. It never merges.

Example spec: [examples/follow-through.md](examples/follow-through.md). The
format and lifecycle are in [orchestrators](../orchestrators.md) and
[RFC 0008](../rfcs/0008-orchestrators.md).

## Choose the right tool

| Need | Use |
|---|---|
| Review, rework and report merge-ready for one session's runs, ad hoc | [`hev expedite`](../expedite.md): the `mine` scope with `say,rework` authority, from your own terminal session, stopped when you return |
| The same judgment for a standing scope (a team's issues), unattended, on a host | This runbook: an `agent: on` loop with a selector |
| Notice that run or PR facts changed, with no model and no judgment | A deterministic watch, `agent: off` with `source: none` (kit#163). On kit main, `agent: off` still requires a source plugin |
| Turn tracker or inbox events into orders | `hev watch` / `hev intake`; not an orchestrator |

A follow-through orchestrator holds no `dispatch` authority and no source, so
it cannot start new work. It acts only on runs that already exist in its
scope.

## 1. Scope selector

`manages.runs` takes exactly one form:

- `{selector: "issue=ACME-*"}` matches every run whose order sets an `issue`
  matching the glob, including runs that start after apply. Have every order
  template set `issue:` so new work is selected without a spec change.
- `{names: [...]}` lists fixed run names (see [carry-over](carry-over.md)).
- `mine` (or `{selector: mine}`) matches runs whose `reply` equals the spec's
  `reply`, which is then required.

A spec with both `selector` and `names` is refused at apply with
`choose one runs selector or names`. `manages.repos` optionally intersects
the scope with repositories; each entry must equal an order's `repo` string
exactly.

## 2. Review by a different harness, then rework

Set the orchestrator's `harness` to differ from the one your workers use
(for example `harness: claude` when orders default to Codex). The round
harness reviews each new PR head against the order's Acceptance. The
implementer's `report.md` is a claim, not evidence.

| Finding | Round does | Authority |
|---|---|---|
| Review findings at head SHA | `hev dispatch rework NAME NOTE...` with every finding in one note | `rework` |
| A question for a still-running worker | `hev dispatch say NAME MSG` | `say` (a `say` that would restart an ended run also needs `rework`) |
| An order with `validation: browser` | `hev dispatch validate NAME`; after a failed verdict, `hev dispatch rework NAME --from-validation` | `validate`, `rework` |
| Clean review, CI green at the same head | Write `reply.md` (section 5) | `notify` |

`validate` launches kit's browser validator on the configured validation
host. Its harness and model pair must differ from the implementer's unless
`--allow-same-model` is given. It is not a code review. If the same
Acceptance item fails validation twice, the run becomes `needs-review` and
further validation-driven rework is refused until a person sends an explicit
rework note.

The loop is bounded by `budget.concurrent_runs` and `budget.runs_per_day`.
Reworks and restarting `say` calls that the orchestrator admits count against
both. Zero means unlimited, so set them.
After a rework, wait for the new head and review it; never review the old one
twice.

## 3. Check CI yourself

The round packet includes PR and CI data that kit gathers. The brief should
still require the round to read checks at the PR's current head, rather than
trusting a worker's "tests pass":

```sh
gh pr view NUMBER --json headRefOid,state,statusCheckRollup
```

Ready means every reported check at that head succeeded. An empty, pending,
skipped, neutral or failed check set is not green. A missing `gh` or an
access error is reported as a blocker, never treated as success. A round's
`gh` needs a token in the supervisor's environment; see
[Windows host: credentials](windows-host.md#2-credentials-at-launch-time) for
launch-time wrappers.

## 4. Named waits in the checkpoint

Every round writes `checkpoint.md` with goal status, open threads by run
name, named waits with release conditions, next action and notes for
`reply`. A wait names what it waits for and how the round will know it is
over:

```markdown
## Named waits

- fix-login-timeout: wait for rework r2 to finish. Release when the run is
  terminal and its PR head differs from 3f9c2e1.
- docs-install-refresh: wait for CI. Release when every check at head
  a41b07d has completed.
- billing-export: wait for a person. Release when the inbox answers whether
  the CSV header may change.
```

With `idle: skip`, a round whose evidence and inbox are unchanged since the
last accepted checkpoint does not call the model, so a wait costs nothing
until its evidence moves. A message (`hev orchestrate say NAME MSG`) wakes a
round early.

## 5. "Ready to merge" for a human

There is no merge authority. When review is clean and CI is green at the same
head, the round writes `reply.md`:

```text
ready to merge: fix-login-timeout PR #42 head 3f9c2e1; review clean (second
harness, high effort); all 7 checks succeeded at 3f9c2e1.
```

Code delivers it to `reply` after accepting the checkpoint, only with
`notify` authority, at most once per round. A person merges, and GitHub's
head match rejects a merge if the head moved. Have the brief keep a "ready to
merge" list in the checkpoint as well, so `hev orchestrate show NAME` shows it.

## 6. Issue-tracker writes (policy; not a kit authority)

**Kit does not implement issue-tracker write authority.** The authorities a
spec accepts are `say`, `notify`, `rework`, `retry`, `validate` and
`dispatch`; any other value is refused at apply. Dispatch admission cannot
see or limit calls a round makes to a tracker.

Some teams want the orchestrator to move an item's status once its work is
verified. Treat that as a policy that a tracker plugin, outside kit, must
enforce on its own side:

- **Allowed:** status moves between named statuses on verified evidence, and
  an evidence comment on the moved item (run name, PR, head SHA, checks).
- **Never:** moves into a terminal or administrative status such as
  Released, Canceled or Duplicate; changing assignment or any field other
  than status.
- **Verified evidence is gathered by the plugin's code**, at the current PR
  head, immediately before the write (for example validation passed, every
  check succeeded, PR merged). The model may request a move; it does not
  supply the evidence.
- **Granted only in private configuration.** The grant, the allowed status
  pairs, the deny list, the credential and a per-day cap live in the
  plugin's own configuration on the host, never in a spec, brief or public
  repository. Start in a dry-run mode that records intents without writing.
- **A condition the plugin cannot prove stays manual.** The round keeps it
  as a named wait and asks a person in `reply.md`.

Without such a plugin, leave tracker writes to people: the round reports
"ready to move" in `reply.md` alongside "ready to merge".

## 7. Lifetime and operation

A loop needs `every` and a positive `max_lifetime`. `until: never` runs until
`max_lifetime`; at expiry it completes and the owner gets a lifetime notice.
To renew, `hev orchestrate stop NAME` and reapply the spec: a new lifetime
starts with the checkpoint and round numbering preserved.
`handoff.rotate_after` bounds checkpoint drift.

```sh
hev orchestrate apply -f follow-through.md --dry-run
hev orchestrate apply -f follow-through.md
hev orchestrate show acme-follow
hev orchestrate say acme-follow 'billing-export: the CSV header may change'
hev orchestrate pause acme-follow
hev orchestrate stop acme-follow
```

On a host without peers, set `notify_route: pull`; see the
[Windows host runbook](windows-host.md#4-owner-notices-for-a-host-without-peers).
