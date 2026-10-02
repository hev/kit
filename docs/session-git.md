Session git capture
===================

`index.GitEnricher.Enrich(ctx, *trace.SessionRow, []trace.Turn)` is the reusable
capture API. `NewGitEnricher()` supplies a git/gh runner; `Run` can be replaced
for tests. Every call has an eight-second budget, shortened by a caller's
context deadline. hevd uses this through ordinary session indexing; a backfill
can enrich stored rows without turns or supply parsed turns for SHA recovery.
The method merges evidence into the row and performs no archive writes.

The `-sessions` contract is:

| Go field | Attribute | Type | Unknown | Filter |
| --- | --- | --- | --- | --- |
| `Workdir` | `workdir` | string | empty | Eq |
| `Commits` | `commits` | []string | empty array | ContainsAny |
| `PR` | `pr` | string, decimal PR number | empty | Eq |

Stores without array attributes encode commits as a filterable JSON string;
these support scalar matching rather than ContainsAny. Reconciliation preserves
existing types and index settings, enabling filters without row rewrites or
namespace deletion. Newly introduced fields are declared on ordinary writes.

Commit IDs are full lowercase SHA-1 or SHA-256 object IDs, unique and sorted.
Git log uses the recorded local branch, workdir and inclusive session window,
with commit timestamps at git's one-second precision. This is temporal branch
evidence: concurrent writers to the same branch within the same window cannot
be distinguished. A missing/deleted branch never falls back to HEAD or all
refs. Full IDs from successful, matched git commit tool results remain usable
when the worktree disappears; abbreviations require unique local commit-object
resolution. Text outside a matching tool result is not commit evidence.

PR lookup uses the recorded repository and branch with `gh pr list --state all`
(limit 20), requiring a captured commit to appear in exactly one returned PR.
This handles merged/closed PRs and deleted local branches without guessing from
a reused branch name. Truncated histories, rewritten commits, unavailable gh,
missing authentication and network failures leave PR unknown. Explicit repo
lookup does not require the worktree to still exist. SSH/HTTPS remotes normalize
to host/owner/repo without forwarding URL credentials.

Rescans union stored commit IDs and retain a known PR when fresh capture has
none, even if a new harness title is present. Unchanged transcripts are skipped
by the existing index cache; resolving a PR created after a session stops needs
a later enrichment/backfill sweep. Outcome and backfill parts own that sweep,
CI/revert fields and measured coverage. Agent-Session trailers are deferred.
