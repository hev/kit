Session outcomes
================

`hev outcomes --namespace archive --days 60 --limit 10 --after ID` processes
one bounded ID-ordered page. Omit `--after` for the first page. JSON output has
`next` and `updated`; continue with `next` until it is empty. On a write error,
output also includes `error` and the last acknowledged cursor; exit status is
nonzero. Retry that cursor. No prompts, blocks, vectors, summaries or evals are
rewritten. Missing PRs use the capture API to retry late PR discovery, and newly
found commits are unioned into the patch.

The reusable Go entry point is
`index.SweepOutcomes(ctx, client, index.NewOutcomeEnricher(), after, sinceMS, limit)`.
Page limits are 1–1000; callers should use small pages and a deadline. Each
observation has a 20-second budget, each GitHub endpoint at most ten 100-row
pages, and all archive requests honor the caller's context. Truncation, missing
authentication, errors and inaccessible histories remain unknown and retryable.
`OutcomeEnricher.Run` and `Now` can be replaced for offline fixtures.

hevd runs a ten-row sweep after ingestion with a 45-second deadline and a
60-day lookback. Its independent, in-memory ID cursor wraps at the end. It does
not use transcript signatures, so unchanged sessions receive late PR, CI,
merge and revert observations. A restart starts the pass again. This cursor is
not a durable replay checkpoint; the returned CLI cursor advances only after a
successful patch (or an unchanged observation). An unchanged observation does
not write merely to refresh its timestamp. For a durable backfill, record the
returned cursor and a fixed cohort/lookback externally; revisit the cohort while
PRs or CI are pending or the observation window is unexpired. Older sessions
can be observed by increasing `--days`. This interface is for an agreed sole
archive writer; the code does not authorize a live replay or daemon restart.

Field and filter contract
-------------------------

Every following field lives on `-sessions` and is filterable. Times are epoch
milliseconds. Strings use `Eq` (including the literal strings `"true"` and
`"false"`); times use `Gte`/`Lt`. Empty attributes on pre-sweep rows are unknown.

| Attribute | Values / meaning |
| --- | --- |
| `pr_state` | `open`, `merged`, `closed` (unmerged), `unknown` |
| `pr_merged` | `true`, `false`, `unknown` |
| `pr_closed` | `true` for merged or closed, `false` for open, `unknown` |
| `pr_url` | GitHub PR link; empty when unavailable |
| `ci_state` | `complete` (collection complete, may include failures), `pending`, `unknown` |
| `ci_conclusions` | Sorted `actual workflow name=conclusion` tokens |
| `ci_runs` | Sorted `name=conclusion\|run ID\|attempt\|URL` tokens |
| `reverted` | `true`, `false`, `unknown`; any linked commit or merge/squash commit |
| `revert_state` | `observed`, `observing`, `complete`, `unknown` |
| `revert_commits` | SHAs of observed revert commits |
| `revert_until` | Merge time + 14 days; zero when unavailable |
| `outcome_checked` | Time of the last persisted observation |

Workflow tokens keep green and red workflows separate. They cover Actions runs
for the current PR head and, after merge, its merge/squash SHA. Paginated results
select the highest run ID and then highest attempt **per workflow ID**, preserving
workflows with the same name. The returned run's actual workflow name is stored.
A noncompleted run has conclusion `pending`; a completed run with no conclusion
is `unknown`. The observed conclusion includes GitHub's `failure`, `cancelled`,
`skipped`, `neutral`, `timed_out`, etc.; these never become `success`.
`ci_state` describes collection/readiness, not an all-green verdict. Pending
takes precedence over unknown in the readiness aggregate; individual fields
remain unknown for unavailable observations.

Array stores support `ContainsAny` on the three lists. Legacy scalar stores
encode lists as JSON strings and support exact whole-value matching. Use the
following stable **scalar** fields for individual outcomes on either store:

* `ci_workflow_<SHA256 of UTF-8 workflow name>_<workflow ID>_name`
* the same prefix with `_conclusion`, `_run`, `_attempt`, `_url`, `_sha`
* `commit_outcome_<full linked SHA>_reverted`: `true`, `false`, `unknown`
* `commit_outcome_<full linked SHA>_revert`: observed revert SHA

`index.WorkflowAttribute(name, workflowID)` computes the workflow prefix. For
example, filter `[prefix + "_conclusion", "Eq", "failure"]` and combine it with
`["pr_merged", "Eq", "true"]` using `And`. The SHA name hash preserves punctuation
and avoids collisions from slug normalization; workflow IDs disambiguate equal
names. Each scalar has its own explicit string/filterable schema declaration.
Names, links, identities and conclusions can be queried without decoding JSON.
On unavailable lookups, previously known dynamic conclusions and per-commit
reverted values become `unknown`; retained run links/identities refer to the last
known evidence and do not imply its current result. Historical attribute keys
remain present and unknown if a workflow disappears or changes name.

Reverts and preservation
------------------------

The window starts at GitHub's PR merge time and ends exactly 14 days later.
The sweep scans paginated retained base-branch history within that interval,
matching standard `This reverts commit <full SHA>` messages, including merge
reverts. It observes original captured SHAs and the merge/squash SHA. Reverting
a revert still counts as a revert having occurred within the window. A seen
revert is immediately `true`; no revert is `unknown`/`observing` before the
window expires and `false`/`complete` only after a complete history scan at or
after the deadline. Open and unmerged closed PRs have no merged observation
window and remain unknown for reverts. An unavailable or capped history never
produces false. Manual inverse changes without the standard message cannot be
identified; negative results describe recognized reverts in retained history,
not proof of semantic equivalence or recovery of force-pushed-away history.

Full session rescans preserve populated outcomes, including flattened fields,
even when the title changes. Enrichment patches update only outcome fields,
PR and unioned commit evidence. On the same host/configuration path,
`.sessions.lock` serializes their read/merge/write with `WriteSessions`; a busy
writer makes the sweep retry rather than block indefinitely. The archive
migration lock is separate, so an upgrade can still write sessions. Multiple
hosts/configuration paths, older binaries and external writers do not share
this lock. Agree on a sole writer for cross-host backfill; no distributed CAS or
atomic array union is claimed. Schema declarations retain existing list types.
No namespace deletion or rebuild occurs.

The read-only export, resumable preview, reconciliation and filter-plan CLI is
documented in [session-backfill.md](session-backfill.md). Its live apply stays
held while the cross-writer protection dependency is unresolved.
