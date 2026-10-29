# Indexed attribute cut (2026-10-29)

Candidate only. No live schemas or documents were changed by this worker. FOH
validates and applies. Keep snapshots and patches private: they contain archive
content. All commands below run from the kit root with FOH's existing authorized
`LAYER_ENDPOINT`, `LAYER_API_KEY`, and puff configuration; no new credentials are
needed. Do not run against a different archive.

`policy.json` and `internal/layer/schema_policy.go` define the exact final set.
Every other declared attribute is explicitly `filterable:false`. Text FTS,
workdir FTS, and embedding configuration stay intact. IDs are provider built-ins,
not additional attribute indexes. Data remains on documents. The two compact
string attributes contain JSON: `outcome_details` maps stable workflow/commit
keys to values, and `marks` maps mark names to integers. Those internal JSON keys
are not namespace columns. Readers accept legacy outcome columns during rollout,
prefer compact observations, and ignore null legacy values. Writers never mirror
observations back into legacy columns. Eval predicates run on the latest eval's
marks JSON in the dashboard process, including missing-mark exclusion.

| Namespace | Starting core | Justified additions and current use | Expected filterable count |
| --- | --- | --- | --- |
| hev-traces-sessions | session_id, harness, host, project, model, start, instance, pr | end: dashboard window and outcome/backfill scan; repo_url: dashboard project filter (project is not populated on old rows); tool_names: dashboard ContainsAny; tool_count, total_tokens, cost, wall_ms: dashboard range filters; commits: backfill empty-linkage conditions; workdir, branch: source/linkage equality guards and existing CLI reconciliation; outcome_checked: conditional stale-observation guard | 1,019 → **19** |
| hev-traces | session_id, harness, project, ts | plan, workdir: `hev find` Eq filters; workdir and seq: `ListSessions` group_by; seq, part: transcript chunk ordering | 28 → **8** |
| hev-traces-blocks | session_id | seq: `SessionBlocks` rank_by | 24 → **2** |
| hev-traces-evals | session_id | none; role, instance, poor and mark ceilings now evaluated locally | 24 → **1** |
| hev-traces-instructions | path, project | host: `InstructionVersions` host-scoped Eq lookup | 10 → **3** |

The sessions result is 19, above the approximate ten-field target, because eleven
existing query fields outside the eight-field starting core remain supported.
No outcome-state or per-value field is retained just in case. A future change
could move dashboard range/tool predicates to the local scan to remove six more
indexes, but that is outside this focused cut. Session project/instance remain in
the requested core even on archives where they are not yet populated.

Filter audit: `internal/serve/filters.go` supplies session model/harness/host In,
repo_url In, tool_names ContainsAny, metric ranges, start bounds; `server.go`
supplies end windows and session_id joins. Stats/value facets use the same local
archive predicates and counts, without additional group indexes. `evals.go` now
filters cached latest evals locally. `search.go` constrains IDs/session IDs;
`internal/layer/client.go` uses ID cursors, session_id prefix ranges, start sort,
chunk ts ranges and the group/order fields above. `instructions.go` uses path/host;
`redaction.go` uses session_id/ID; outcome/backfill code uses end/start and ID scans,
linkage and stale-write fields above. `cmd/hev/find.go` adds plan/workdir/harness/ts;
CLI git/enrichment reconciliation retains commits/pr/workdir. Row-scoped write
conditions compare source/outcome values to prevent races; they do not introduce
additional scan/facet indexes. Dynamic mark query parameter names are parsed for
validation only and never sent to Layer.

The reported live baseline has 1,025 session attributes, including 812
commit_outcome_* and 126 ci_workflow_* columns (938 dynamic outcome columns).
Nulling values does not necessarily remove schema definitions, so attribute
counts can remain 1,025 or grow for missing compact/core declarations while
filterable counts fall to 19. The exact row counts, dynamic column counts, and
non-null values to clear are computed from the snapshot in `report.json`. Evals
may have a variable number of mark_* columns; the planner inventories all of them.

## Preview and review

First arrange an exclusive archive maintenance window: pause ordinary writers
through the normal FOH process, and keep them paused through snapshot, apply,
validation and any rollback. An ID scan is not a point-in-time transaction.
Deploy the candidate's writers/readers as part of the approved rollout so old
binaries cannot recreate flattened fields or restore broad indexes. The worker
has neither paused writers nor deployed this candidate.

The checked-in five `*.schema.json` files are complete **declared baseline**
schemas without embedding redeclarations. They cannot enumerate unseen live
columns; do not apply them directly as the live repair. Compile against an actual
snapshot to include every historical and unknown column, retain its type and FTS
options, and explicitly disable it unless listed in policy. Existing definitions
win over baseline types (including string versus array storage).

```bash
# Read-only network step, FOH only; exports every physical revision, not latest only.
python3 docs/schema-cut/snapshot.py --output /private/kit-schema-before

# Entirely offline. Output directory must not already exist.
python3 docs/schema-cut/plan.py \
  --snapshot /private/kit-schema-before --output /private/kit-schema-plan
cat /private/kit-schema-plan/report.json
python3 -m unittest discover -s docs/schema-cut

git diff --check
go test ./...
```

A successful plan is required before applying any files. Errors leave an
incomplete output directory for diagnosis; discard it and rerun to a new output
path. Malformed JSON, duplicate row IDs, or conflicting compact versus legacy
values stop planning. Resolve conflicts explicitly in a copy of the snapshot,
retain the original export as evidence, and review the resolution. Never null a
legacy value that has not been folded. Null-only legacy columns need no row
rewrite, but their schemas are still disabled. Replanning already repaired rows
produces zero row patches. No planner code can make network requests.

Review all five generated schemas and the patch/rollback JSONL side by side.
Each JSONL line is a complete bounded `patch_rows` request for one physical ID.
Only `outcome_details`/`marks` and the old dynamic attributes change; text,
embeddings, summaries and other attributes are absent from row patches. The
planner retains backups of every changed value, including original compact JSON.

## Apply (FOH after validation only)

The generated schema files are raw puff schema JSON, not a write envelope:

```bash
set -euo pipefail
for ns in hev-traces hev-traces-sessions hev-traces-blocks hev-traces-evals hev-traces-instructions; do
  puff schema apply -n "$ns" < "/private/kit-schema-plan/$ns.schema.json"
done
```

If the installed puff build takes schema JSON as a positional file rather than
stdin, use its documented file argument with the same generated JSON. The puff
binary is absent from this worker environment; its CLI invocation could not be
exercised here. The equivalent wire request, with no dependency on puff's input
convention, is exact:

```bash
for ns in hev-traces hev-traces-sessions hev-traces-blocks hev-traces-evals hev-traces-instructions; do
  jq '{schema:.}' "/private/kit-schema-plan/$ns.schema.json" |
    curl --fail-with-body --silent --show-error \
      -H "Authorization: Bearer $LAYER_API_KEY" -H 'Content-Type: application/json' \
      --data-binary @- "${LAYER_ENDPOINT%/}/v2/namespaces/$ns"
done
```

Wait for index changes to become ready before checking dependent queries (the
provider may return 409 while rebuilding). Then apply the compact row patches:

```bash
for ns in hev-traces-sessions hev-traces-evals; do
  while IFS= read -r body; do
    printf '%s\n' "$body" |
      curl --fail-with-body --silent --show-error \
        -H "Authorization: Bearer $LAYER_API_KEY" -H 'Content-Type: application/json' \
        --data-binary @- "${LAYER_ENDPOINT%/}/v2/namespaces/$ns" |
      jq -e 'if (.error // "") != "" then error(.error) elif .rows_affected != 1 then error("expected one affected row") else . end'
  done < "/private/kit-schema-plan/$ns.patches.jsonl"
done
```

Inspect each response for API errors and exactly one affected row; stop on any
mismatch. Capture stdout in the maintenance evidence. Resume an interrupted
rewrite only while writers remain paused: each patch is idempotent. Re-export to
`/private/kit-schema-after`, replan to a fresh directory, and require zero
`rewrite_rows` and the expected 19/8/2/1/3 indexed counts. Compare physical ID sets,
row counts and unchanged fields with the original snapshot. For each rewritten
row, check all original dynamic values exist in compact JSON and all old values
are null. Smoke-test dashboard mark ceiling, role, instance, poor and project
filters, outcome display/CLI reads, and `hev find --plan`/`--workdir`, then compare
a 30-day dashboard request's Layer timing. No live performance result is claimed
by this candidate.

## Rollback (same exclusive maintenance window)

Apply the generated `*.rollback.jsonl` using the same row-patch loop, substituting
`rollback` for `patches`. Apply all five `*.rollback-schema.json` with the schema
loop. This restores the original filterability (including implicit defaults),
original compact values and every nulled legacy value. Newly introduced
attributes remain declared but unindexed, since schema deletion is unnecessary
and may be unsupported. Their row values are restored to null where originally
absent. No text/vector rewrite or re-embedding is involved. Validate against the
original snapshot before choosing which reader/writer version to resume. Do not
roll back over newer live writes; once writers resume, take a fresh snapshot and
reconcile changes instead of blindly replaying the old rollback.
