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

Capture lifecycle and identity
------------------------------

A hevd cycle reconciles query filters, runs the pre-existing session-list
migration check, then calls `index.Run` for each configured transcript source.
Source units are enumerated in key order. An unchanged size/mtime signature is
skipped; otherwise the unit is read and redacted. Ordinary ingestion writes
retrieval chunks, ordered whole blocks, and then aggregate sessions. Capture
runs after aggregation and before `WriteSessions`, with eight seconds per
session for git/gh. A capture lookup failure leaves unknown metadata and does
not fail ingestion. Store/schema errors do fail that unit, leaving it retryable.
There is no periodic capture sweep of unchanged sessions in this part.

`SessionRow.ID` and `SessionID` are both the transcript's session ID. They do
not incorporate workdir, branch, host, source path, or an enrichment timestamp.
Copied/resumed transcripts with the same session ID therefore address the same
row. IDs are not newly namespaced by harness: callers must not combine genuinely
different sessions with colliding IDs in one archive. Tenant isolation comes
from the configured namespace, not from the ID. Blocks/chunks use separate
content-derived IDs; session capture does not change them or write evals.

`WriteSessions` reads the current commits schema on every call. An existing
`string` declaration uses JSON-string encoding even on an arrays-capable store;
an existing `[]string` declaration uses arrays. Only an absent attribute uses
store capabilities to choose its type. Schema read failures, other types, or
arrays on a store that cannot serve arrays return an error before row writes.
There is no conversion, namespace rebuild, or live repair command in this path.
A schema change by another writer between read and write remains subject to
provider rejection and retry; this read is not a schema transaction.

Pagination, high-water marks and checkpoints
-------------------------------------------

These APIs exist today, with the following limits:

| API/state | Implemented semantics | Replay limit |
| --- | --- | --- |
| `State.Units`, `LoadState`, `State.Save` | Successful source-unit signatures; scoped to archive target, credentials/model/store, host and source root for built-in sources. hevd saves successful signatures at cycle end, including when another unit failed. | Ingestion cache only; no session enrichment cursor or outcome revision. Save uses a normal file write, not an atomic replay journal. |
| `Options.Force` | Reprocesses unchanged units. A signature is recorded only after the unit's writes succeed. | Still writes blocks and sessions; ordinary mode also writes chunks. It is not a sessions-only enrichment replay API. Existing redaction upgrade/migration paths may also run. |
| `Options.ReadSide` | Avoids normal retrieval-chunk writes while rebuilding blocks and sessions. | Rewrites whole session rows, and is not authorized as a capture-only backfill. |
| `ListSessionRows(-1, filter)` / slim variant | Ordered scan in pages of 10,000, ID ascending; subsequent requests add `id Gt lastID` to the supplied filter. Rejects a non-advancing cursor; finally sorts returned rows by start descending. | Cursor is internal and not exposed/persisted. No snapshot, fixed upper ID, or durable page acknowledgment. Concurrent inserts below the cursor or rows newly entering the filter can be missed. |
| `SessionsWatermark()` | Returns metadata `last_write_at` for cache invalidation. | Not a session-time cutoff, transactional snapshot token, or completed replay high-water mark. |
| `GitEnricher.Enrich` | In-memory capture merge for one stable session row, optionally using parsed turns. | No pagination, persistence, checkpoint, report, or archive mutation. |

A crash after store writes but before signature save can replay those source
units. Stable IDs and the end-time upsert condition limit ordinary duplication;
they do not provide exactly-once enrichment. An unchanged transcript that later
gains a PR will remain skipped until a separate sweep explicitly revisits it.

A bounded 60-day sessions-only replay command, dry run, resumable checkpoint,
fixed cutoff/high-water definition, page iterator, acknowledgment/retry policy,
and coverage report are **not implemented by capture**. The dependent replay
owner must establish them before applying. A proposed checkpoint should bind
target/schema and selection boundaries, retain stable IDs and observed versions,
and advance only after acknowledged safe writes; that is a requirement for the
later design, not a claim that such an API already exists.

Preservation and concurrent writers
-----------------------------------

`WriteSessions` is a whole-row upsert. It unions stored commit IDs, retains a
stored PR/workdir when incoming values are empty, and retains a stored summary
when no new title is supplied. It does not generically retain unknown attributes.
Outcome fields and flattened workflow/commit attributes now have explicit
preservation support; unrelated fields
must not be copied into a replay and then overwritten from a stale snapshot.
Where conditional writes are served, the upsert condition accepts missing `end`
or stored `end <= incoming end`.
It prevents older transcript-end rollback, but equal end times remain writable
and it is not compare-and-swap over enrichment versions.

For example, ingestion reads commits A, an independent sweep patches commits B,
then ingestion upserts its row with A: B can be lost despite the preservation
read. Patching only enrichment fields avoids overwriting unrelated fields in
the sweep, but cannot prevent a later stale whole-row ingestion upsert. Concurrent
sweeps that replace arrays can also lose each other's additions. The outcome sweep adds a local `.sessions.lock` around `WriteSessions` and
`PatchOutcomes`, covering direct client calls on the same configuration path.
The patch unions fresh and stored commits under that lock. This closes the local
read/upsert race described above. There is no distributed lock, atomic union or
enrichment CAS across hosts. `PatchSessionSummaries` remains summary-only.

Built-in `index.Run` uses a local archive lock at the configured redaction
config path plus `.archive.lock`; hevd also has a local daemon lock. Neither
coordinates different hosts/config paths, direct `Client.WriteSessions` calls,
or independent network writers. Worker parallelism is bounded, not a global
session-write serialization guarantee. A replay must agree on a sole writer or
implement a protocol honored by incremental ingestion and all enrichment writers
before overlapping them. It must preserve unrelated attributes and evals. Do
not restart shared daemons, invoke competing replay, or treat capture's local
locks as authorization to mutate a shared archive.

Capture and its dependent git/PR/CI/revert sweep concern `-sessions` enrichment.
Outcome labels and `-blocks` to `-evals` labeling are separate work owned by
the label pipeline. This part provides no label implementation, eval replay,
live archive mutation, or live coverage receipt. Operator coordination and the
later sessions-only replay owner's agreement are prerequisites to any apply;
private archive material and identifiers belong outside this public repository.

See [session-outcomes.md](session-outcomes.md) for the bounded sweep, filters,
cursors and observation semantics.

### Ingestion preservation

For an existing session, `WriteSessions` conditionally patches source metadata
instead of replacing the row. It omits summaries, outcome observations and
workflow details. Capture linkage merges current commits and preserves populated
PR, repository, branch and workdir values; exact current linkage and source-end
conditions reject concurrent fills or source advances. A conflict gets a fresh
read and at most three write attempts. A newer stored source is left intact.

Creation uses an ID-absent conditional upsert. If another writer creates that ID,
ingestion reads the winner and follows the existing-row path. Successful writes
require exactly one affected row and persisted source readback; optional affected
ID lists are checked when the provider returns them. Existing schema definitions,
including legacy string-encoded lists and filterability settings, are retained.
An existing-row update needs conditional patches (Postgres gateway 0.7.2 or newer).
Unsupported stores fail without falling back to a whole-row overwrite.

This protects writes made by this client. Older installed clients and direct
whole-row publishers can still erase enrichment; updating code does not activate
it in a running daemon or establish distributed writer coverage.
