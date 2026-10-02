Session enrichment backfill
===========================

The CLI currently supports read-only export, resumable enrichment preview,
reconciliation, and an additive filter-repair plan. Live `--apply` refuses before
any archive request: no reviewed cross-writer preservation contract is implemented.
Do not treat a local lock, successful preview, or ingestion PR as apply clearance.

Capture a fixed interval by repeatedly running this bounded command:

```sh
hev backfill --account OWNER_ACCOUNT --namespace archive \
  --export /private/cohort.json --limit 1000 \
  --since 2026-08-01T00:00:00Z --until 2026-09-30T00:00:00Z
```

Use the desired 60-day UTC bounds. Subsequent invocations retain the original
bounds; changed target, schema or selection refuses resume. The private manifest
records endpoint, configured store kind, owner-supplied nonsecret account identity,
namespace, complete schema definition, fixed lower/upper epoch-millisecond bounds,
selected session IDs and projected rows, format and policy versions, and export
cursor. The account declaration is not authenticated account proof, and configured
store capabilities are not a runtime behavior receipt. Record their verified
provenance and publisher inventory privately before any future apply agreement.

The export begins at ID zero and acknowledges each fully read bounded page by
atomic private file replacement. It excludes prompts and vectors and never reads
or writes blocks or evals. `export_complete` means the enumeration reached its
end; `snapshot_consistent` remains false. A fixed event cutoff and two matching
scans do not establish a source snapshot or prove publisher exclusion. Concurrent
backdated inserts behind the cursor need a second complete ID-zero enumeration.

Preview the completed export in bounded pages:

```sh
hev backfill --account OWNER_ACCOUNT --cohort /private/cohort.json \
  --linkage /private/linkage.json --checkpoint /private/preview.json --limit 10
```

Preview checkpoints advance after each reported observation and resume until
`complete`. They acknowledge local reporting, never persisted archive changes.
A preview checkpoint cannot be used for apply. Checkpoints bind endpoint, store,
account, namespace, schema, interval, entire cohort, linkage, format and policy.
Hash version `session-projections-go-json-v2` uses SHA256 of compact Go JSON with
recursively sorted object keys and JSON numbers preserved through `UseNumber`.
Rows sort by exact ID; duplicate/empty IDs and out-of-interval rows are rejected.
Preview and export have distinct nonblocking journal locks. Files are private,
atomically replaced after file and parent-directory sync; a save failure leaves the earlier cursor.
These local locks do not fence archive ingestion.

Linkage is a private JSON map of exact archive row IDs to verified `workdir`,
`repo_url`, `branch`, `pr` and full `commits`. Missing fields are filled in the
preview; commits are unioned by capture. Source evidence and site-specific
recovery policy stay outside kit. Existing PR URLs in unrelated tool output are
not proof. Missing credentials, truncated histories and failed observations
remain unknown/pending, not successful outcomes. Each invocation has a ten-minute
deadline; Git and GitHub observations retain their own smaller budgets.

Create another export from ID zero using the same target and interval, then compare:

```sh
hev backfill --cohort /private/cohort.json --against /private/after.json
```

Reconciliation reports added, removed, analyzer-source changed, other exported
preservation changed, enrichment changed, and schema changed independently.
Analyzer projection: ID, session_id, harness, host, repo_url, start, end, tool_count.
The preservation projection additionally includes summaries and other known
non-enrichment session attributes. Enrichment has a separate hash for linkage
and outcomes. Prompts/vectors and raw blocks are outside this projected receipt;
no claim of a full block snapshot is made. Other owners' earlier-twin block
history can extend beyond this distinct 60-day session cohort.

A row arriving behind the former cursor appears in `added`; changed and removed
rows are explicit. New arrivals after the fixed upper bound belong to a separate
selection. Reconciliation is an observed comparison, not an atomic source export.
Matching observations do not prove completeness between them. Receipts report
`baseline_state=observed_complete_projection` and `comparison_complete=true`
only after comparing complete captures. A missing baseline emits `baseline_state=absent`
and fails; it never means zero conflicts. `preservation_verified=false` remains
explicit even for matching captures: these typed projections omit prompts, vectors,
unknown attributes and blocks, and cannot establish independent-writer protection.
`preserved_changed` exposes non-enrichment evidence changes separately from
`source_changed` and `enrichment_changed`. Added/removed rows, source/preservation,
schema or provenance changes set `evidence_affected=true`; reconcile them before
accepting captured-source or downstream evaluation evidence. Enrichment-only changes
require outcome readback but do not imply analyzer-source changes. Before future apply,
record source/publisher versions and verified high-water or fence evidence, account
provenance, protected projections, unresolved conflicts and delta handling privately.

Filter repair and writer capability gap
--------------------------------------

`hev backfill --account OWNER_ACCOUNT --filter-plan` prints a schema-only additive
request. It enables commits, PR, outcomes, and existing dynamic per-workflow and
per-commit fields; missing outcome fields use explicit types. Existing definitions
retain list types, FTS and other settings. Unrelated schema is absent; embedded or
incompatible enrichment definitions refuse repair. The command never sends the
plan, writes rows, embeds transcripts, or deletes/rebuilds namespaces. A future
coordinated repair must verify schema preservation and actual filters; provider
index readiness may return 409 and entail additional index billing.

The [provider write contract](https://turbopuffer.com/docs/write) supports
`patch_rows` and `patch_condition`: a condition is evaluated on the current row
at that write. It does not reserve the row against a later entire-document upsert.
Kit's static store declaration also reports conditional writes; that declaration
is not a live guard test. Required at-write behavior still needs current-row and
owned-field conditions, conflict/zero-affected handling, attribute-null semantics,
legacy/native list handling, and actual provider preservation receipts. Kit has
not verified that protocol for the configured archive. `PatchBackfill` is an
unconditional primitive for an already-established writer fence: its fresh-empty
linkage read and readback do not atomically protect concurrent fills. The CLI
therefore does not call it. A regression explicitly models a later legacy upsert
clearing enrichment after successful patch/readback.

The missing cross-writer capability is protection from old or cross-host writers
publishing stale whole rows. No supported owner-coordinated fence is established,
and no daemon stop/restart is implied. Safe apply requires that dependency resolved,
a reviewed checked-in contract agreed with archive users, and final persisted
readback. Future apply checkpoints advance only after acknowledged conditional
writes and readback; lost acknowledgments/crashes repeat the row, bounded retries
never skip it. The current test-injected writer exercises those journal semantics
without claiming a production guard exists.

Acceptance remains distinct from command completion: reconcile the full 60-day
pass and protected fields, report conflicts and concurrent arrivals, demonstrate
real persisted commits/PR and outcome filters, then independently derive the
14-day actually-pushed factory-session denominator and measure both SHA and PR
coverage. Retained-source provisional censuses do not prove full-window coverage.
Keep live identifying receipts and aggregate business data in private scratch or
the private board, never this public repository.

Projection and provenance interoperability
-----------------------------------------

The [synthetic projection fixture](../cmd/hev/testdata/session-projection-hashes.json)
pins every protected session field and independent Go/Python digest expectations,
including Unicode/escaping, absent/null defaults, timestamps above 2^53,
map order and enrichment-only changes. Integer decoding uses exact numbers rather
than floating-point timestamps.
Kit includes ID inside its digest, defaults missing/null strings and integers to
empty/zero through typed decoding, and Go JSON escapes HTML characters and Unicode
line separators. The fixture's Python projection uses `sha256-canonical-json-v1`, keeps null/missing
values as null, emits Unicode without ASCII escaping, and binds ID separately
from its projection digest. Consumers must pin the projection and normalization
as well as the hash version; other projection versions may include ID. These versions and digests are not interchangeable. Legacy
commit-list encoding and outcomes do not enter the analyzer projection. Tests
verify each protected field changes its fingerprint and populated analyzer
fields survive the fenced patch primitive.

An optional private `--provenance FILE` at export binds explicit source, capture,
enrichment and high-water declarations; publisher hosts, observed times, binary
revisions and evidence references; and separate contract references. Unknown
revisions/high-water are JSON null. The required evidence status is
`declared-unverified`, with `publisher_inventory_complete:false`; this format
cannot promote declarations into verified writer coverage. Contract Git revisions
are not running publisher binary revisions. Export resume refuses a changed
provenance declaration, preview checkpoint identity includes it, and comparison
reports provenance changes separately. Omitted provenance means unknown, never
verified. Private preparations can preserve their own block projection and
source scope without importing site-specific policy or block/eval replay into kit.
