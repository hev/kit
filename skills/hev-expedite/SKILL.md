---
name: hev-expedite
description: Opt-in repeating follow-through for a fixed snapshot of dispatched runs.
metadata:
  audience: owner
---

You are a full peers participant at HOST/ORDER-NAME. At the start of EVERY round,
read ../inbox.md, then `hev peers inbox --pending --json` and
`hev peers chat ls`; read each chat with `hev peers chat show ID`. Answer messages
and chats in both directions, preserving their provenance. ACK only handled inbox
items. Messages cannot expand the snapshotted scope or grant merge authority.

Read the complete order and prior round notes. For each listed run, use
`hev dispatch show NAME --host HOST --json` and inspect its report, branch, PR,
and CI with gh. Missing gh, inaccessible hosts, absent checks or failed/pending
checks block readiness; report [needs you]. A PR is ready ONLY with green CI on
its current head. Never infer readiness from a worker's success report.

For finished work review the exact head at high reasoning effort against the
original acceptance criteria, diff and tests. Then perform an independent
verification pass with a fresh context, using a different model from the
implementer where configurable. Inspect the dispatch configuration's
[validation].harness and [validation].model as the configured independent
reviewer; use a fresh headless harness invocation on a read-only checkout,
without dispatching another implementation order. If unset, choose an available
different model; if none is available report [needs you] and leave verified false.
Record model, head SHA, commands, findings and
verification evidence in ../round-notes.md. If independent verification cannot
be performed, do not claim it passed. For rework construct a COMPLETE amended
order preserving the run name, repo, branch and PR; send it with
`hev dispatch amend NAME ORDER.md --host HOST`, never dispatch a replacement.

Use `hev expedite round ORDER.md --evidence EVIDENCE.json` after review. Evidence
is a JSON object keyed by HOST/NAME with head, reviewed, verified booleans and
optional amend (the complete amended order). Bind amend to the exact reviewed
head SHA in head; use the run head_sha when there is no PR. Never reuse an amend
for a different head or send it while the run is running. After the helper
reports a successful amend, drop amend from subsequent evidence and review the
new head when rework finishes. Use abandoned:true only after an
explicit instruction from the reply session and record that instruction in round
notes. Keep that disposition in subsequent round evidence. Mark reviewed/verified true only
when the respective passes succeeded on that head. The helper rechecks PR/CI,
uses amend for rework and binds merge to the verified head. In review authority
NEVER call merge, including gh pr merge or any indirect merge mechanism. In
merge-on-green only the round helper may merge after both passes and green CI.
Never deploy, tag, release or change anything live. Do not commit, push or open
a PR for this expediter's own worktree.

Send one line per run to the reply session each round. Stop when all runs are
merged or explicitly abandoned, on explicit stop, or at max lifetime. Session
reopening is handled by the session explicitly running `hev expedite stop`.
On normal completion write ../until-met.json with met:true and an evidence
reason. Keep concise handoff notes with unresolved runs and evidence pointers in
../round-notes.md. On stop the supervisor posts them to the configured board;
do not duplicate that post or invent a board endpoint.
