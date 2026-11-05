---
name: hev-handoff
description: Hand off dispatch results with evidence. Use when completing or resuming a worker run, reporting acceptance results, failures, blockers, and remaining work.
---

Keep status short and current. In ../report.md record the revision, PR URL, every acceptance result, exact failing or flaky test names, blockers and remaining work. Include reproduction steps and evidence paths. Never claim unrun checks passed. Inspect the durable order before continuing another worker’s result.

```sh
hev dispatch show example-order
hev dispatch logs example-order
hev validate example-order
```

For unfinished work, leave a compact pointer in the existing issue or shared
handoff thread: current state, first concrete next action, open work, pushed
branch/PR and revision, decisions, failed approaches, blockers, and trace ID.
Link evidence instead of copying logs or retelling a transcript. Search for an
existing handoff before creating another; update the same thread as work moves.
If the receiving worker is already running, send it the pointer through its
inbox. A delivery receipt does not mean the worker has read or accepted it.

On pickup, read the full order, report and later inbox entries. Check the live
issue, branch and PR before trusting the old note; record who picked up the
work. Close the handoff when its work lands, or update it with the next action
when stopping again. Keep durable project knowledge in the project itself.
If a branch cannot be pushed, explicitly name that blocker and where the work
can be recovered. Dispatch status and report files remain required; other
handoff files should be created only when requested by the user or order.
