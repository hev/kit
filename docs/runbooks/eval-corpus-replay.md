# Replay a historical eval corpus

A gateway region move does not rebuild stored evaluations from transcripts.
Use a bounded one-off replay owned by an operator or factory session. Rebuilding
this namespace in the transcript daemon needs a separate design for authoritative
eval sources, checkpoints, retention and warnings; resetting the index is not a
recovery mechanism for evaluations.

`scripts/replay-eval-corpus.py` preserves the old namespace's stored wire rows,
original IDs and schema. `hev eval put` is appropriate for new evaluations but its
fixed Eval decoder is not a lossless historical migration interface: it discards
unknown fields and derives IDs. Do not adapt historical marks into a new scoring
scale, regenerate IDs, or rewrite timestamps to make migration succeed.

Python 3.11 or later is required. Coordinate with every writer to the destination
namespace first. Historical replay and an outcome labeler must own disjoint IDs;
a conflicting ID stops replay. No service restart or daemon installation is
needed. The script uses the namespace HTTP API, an explicit HTTPS endpoint and
an explicitly named existing config file for each endpoint. It never inherits
credentials or target settings from the environment. Do not change shared
configs or credentials to make this command work.

Create a private mode-0700 receipt directory outside **all** git checkouts. Copy
the source JSONL to a private file to bound the cohort; retain its hash and row
count. Config paths and receipts must remain private. Substitute approved existing
config paths and private source/receipt paths below:

```sh
python3 scripts/replay-eval-corpus.py \
  --source-config /private/old-config.toml \
  --source-endpoint https://gcp-us-central1.turbopuffer.com \
  --target-config /private/gateway-config.toml \
  --target-endpoint https://aws-us-east-1.hevlayer.com \
  --namespace hev-traces-evals \
  --jsonl /private/cohort.jsonl --receipts /private/replay-receipts \
  --max-source-rows 6000 --max-target-rows 100000
```

This preflight performs read-only old/new schema and paginated full-row
inventories. By default the old stored `(session_id, ts)` pairs must match the local
`(session, ts)` identity set exactly. If private inspection confirms different
bounded cohorts, explicitly pass the reviewed `--expected-local-only` and
`--expected-stored-only` counts. Pass `--include-local-only` to recover those local-only evaluations through
the supported Eval wire shape; otherwise they stop replay. Stored-only evaluations
retain their original rows and IDs. Structured local findings become canonical
JSON strings inside the required findings string array, preserving every object
field and value. Marks remain integers, including their filterable `mark_*`
attributes. New IDs use kit’s canonical session/timestamp hash; existing IDs and
timestamp attributes are never regenerated. Additional producer metadata outside
the Eval contract remains in the private source snapshot. Unexpected differences stop replay. Historical source count, planned row count, local unique pairs and local line count
explain growth and duplicate lines separately. The filed historical count is
approximately 5,394; the live source may have grown. Do not equate total target
count with historical coverage when other writers add labels.

Preflight refuses incompatible field definitions (including filterability),
conflicting IDs and inventories over the specified bounds. An empty or missing source and unauthorized reads stop the operation.
An absent destination is recorded as an empty preservation baseline and is
created by the first insert-only batch, without deleting any namespace. It does not reset/delete
namespaces or repair incompatible schemas. Inspect source/target receipts
privately, including `ts`, `instance`, `findings`, `marks` and `mark_outcome`.
Review vector representation if present; no field is intentionally removed from
stored rows except the query's `$dist` score. For local-only rows the destination
may add the embedding attribute declared by the original schema; readback checks
every supplied attribute exactly and permits only that generated field.

After coordination and successful preflight, repeat the identical command with
`--apply`. Only missing rows are written in batches of 30, with the server-side
insert-only condition `id Eq null`. The journal binds both config digests, endpoints, source snapshot and bounds.
A receipt-directory lock prevents concurrent replay invocations. The original
preservation baseline is saved
before the first write. Keep this journal after failures, lost acknowledgements
and restarts: rerunning reconciles the same frozen rows and never replaces the
baseline. A race inserting a conflicting ID is detected by readback. The script
cannot undo another writer's changes and stops if a preexisting row changes.

Acceptance requires complete source row equality by original ID, unchanged
baseline target rows, exact source schema/filterability plus unchanged preexisting
target field definitions, and a query returning the matching ID for every source
filterable field. A same-value existing-row probe must report zero affected rows to verify the
gateway’s insert-only condition without attempting a conflicting replacement.
Run again to prove no missing rows and no new inserts. Receipt
files retain the inventory, full readback and aggregate summary privately; post
only aggregate counts, query result counts, schema comparison and the PR link.
Additional target rows/fields remain intact. Inventory reads are not snapshot
isolated; receipts explicitly record that limitation.

If credential access, schema compatibility, original identity coverage or writer
coordination blocks replay, record the exact blocker in the issue and leave the
recovery acceptance open. Do not broaden into deployment or credential repair.

Run synthetic preservation and lost-ack checks with:

```sh
python3 scripts/test-replay-eval-corpus.py
```
