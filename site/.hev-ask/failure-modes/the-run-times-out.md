---
id: "failure-modes#the-run-times-out"
title: "Failure modes"
heading: "The run times out"
group: "Operate"
order: 228
url: "/docs/failure-modes#the-run-times-out"
anchor: "the-run-times-out"
terms: ["times","supervisor","enforces","order","timeout","default","minutes","kills","harness","descendants","leaving","worktree","branch","state","continue","rework","telling","worker","pick","status","file","cleaned","refuses","dispatch","name","finish","tests","passes","ends","everything","started","stay","timed","confirm","same","verify","running","again","because","stop"]
hash: "11662b410f8b543b61deca5a74e3566b31e906b2a98db721521dd1890c1165ea"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rework NAME 'Pick up from status.md and finish the tests'","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"timeout","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"--timeout","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"hev dispatch run","chunkId":"failure-modes#the-run-times-out"}]
sources: [{"chunkId":"failure-modes#the-run-times-out","url":"/docs/failure-modes#the-run-times-out","anchor":"the-run-times-out"}]
---

The supervisor enforces the order timeout (default 120 minutes), then kills the harness and descendants, leaving worktree and branch with state timeout. Continue via rework telling the worker to pick up from its status file; if the worktree was cleaned, rework refuses.
