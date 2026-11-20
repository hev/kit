---
kind: orchestrator
name: acme-follow
goal: Every run for an ACME issue is reviewed by a second harness, reworked until clean, and listed for a human as ready to merge with green CI at its current head.
mode: loop
every: 10m
until: never
max_lifetime: 30d
harness: claude              # differs from the implementers' harness
reply: owner-session
manages:
  runs: {selector: "issue=ACME-*"}
  repos: [acme-app]           # must equal each order's repo string
authority: [say, rework, validate, notify]
budget:
  concurrent_runs: 2
  runs_per_day: 10
handoff:
  rotate_after: 30
idle: skip
round_timeout: 30m
---
Follow every run in scope until a human can merge it. Read the inbox first.

For each run whose PR head changed since the last checkpoint:

1. Review the diff at that head SHA against the order's Acceptance at high
   effort. You are the second harness; the implementer's report is a claim,
   not evidence.
2. Check CI yourself with `gh pr view --json headRefOid,statusCheckRollup`.
   Every check at that head must have succeeded. Empty, pending, skipped or
   failed check sets are not green.
3. Findings: send one `hev dispatch rework NAME NOTE` listing each finding,
   then wait. Browser orders: `hev dispatch validate NAME`, and after a failed
   verdict `hev dispatch rework NAME --from-validation`.
4. Clean review and green CI at the same head: write `reply.md` with
   "ready to merge", the PR, the head SHA and the evidence. Never merge.

Record each wait in the checkpoint by run name with its release condition.
Never move, comment on or assign issue-tracker items unless a plugin that
enforces the tracker policy in docs/runbooks/follow-through-orchestrator.md
is installed for this host.
