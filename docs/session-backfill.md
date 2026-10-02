Session enrichment backfill
===========================

`hev backfill --checkpoint /private/replay.json --limit 10` reports a bounded
page of the configured archive's last 60 days without writing. Add `--apply`
only after coordinating all archive writers, including older binaries and
other hosts. Local session locks alone do not protect distributed ingestion.
No daemon restart is required by this command.

Apply saves a private, atomic checkpoint after each acknowledged row patch.
Repeat until `complete` is true. The fixed lower time bound and namespace are
recorded in that checkpoint; resume does not slide the lookback. ID pagination
is not a snapshot: concurrent inserts behind the cursor require another pass.
Create a new checkpoint for later observation passes. A failed write retries
at most `--retries` times (0–5), with bounded exponential delay, then leaves the
last acknowledged cursor. Each invocation has a ten-minute deadline and at
most 1000 rows. An interrupted acknowledged write before checkpoint rename
can repeat safely; commits are unioned and enrichment patches are idempotent.

`--linkage /private/linkage.json` accepts a JSON object keyed by exact archive
row ID, with verified `workdir`, `repo_url`, `branch`, `pr`, and full `commits`.
It fills missing linkage and unions commits. Derive this evidence from source
metadata/transcripts outside kit; never guess a PR or use unrelated HEAD
history. Site-specific policy and identifying receipts belong in private
scratch. Dry runs can invoke read-only git/GitHub observations and print private
row data; direct their output to private files. They never create checkpoints.

Apply patches linkage and outcomes only. It does not replay ingestion, rewrite
summaries, embed transcripts, or delete namespaces. Existing list schema types
are retained. Missing GitHub observations remain explicit unknown/pending;
command success is not a coverage claim. Independently enumerate sessions that
actually pushed, join exact archive IDs, report exclusions and missing links,
and test actual sessions filters before claiming acceptance.

Apply additionally requires `--cohort /private/sessions.json`, an immutable JSON
array of exported session rows. Checkpoints bind the endpoint, namespace,
cohort bytes, linkage content and replay policy version; changed input refuses
resume. Each patch requires one affected row and readback of every patched
attribute before advancing the checkpoint. Source linkage fills only attributes
still empty in the fresh pre-write read. Readback detects observed conflicts;
it cannot prevent a later stale ingestion overwrite. Sole-writer coordination
remains necessary and this command does not provide distributed CAS.

An export is a fixed replay input, not proof of a source snapshot. Record the
export cutoff, schema, digest, writer inventory and consistency limits privately.
Compare a second full source enumeration and reconcile added/removed/changed
IDs (including backdated insertions) after the pass. Distinguish protected
transcript/summary metadata from enrichment changes. If writers cannot be
coordinated or preservation cannot be verified, hold apply and report the
blocker. A complete replay checkpoint alone never establishes live acceptance.
