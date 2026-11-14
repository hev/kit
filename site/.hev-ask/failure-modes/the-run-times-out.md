---
id: "failure-modes#the-run-times-out"
title: "Failure modes"
heading: "The run times out"
group: "Operate"
order: 165
url: "/docs/failure-modes#the-run-times-out"
anchor: "the-run-times-out"
terms: ["times","supervisor","enforces","order","timeout","minutes","default","ending","harness","everything","started","while","keeping","worktree","branch","continue","rework","longer","work","dispatch","name","pick","status","finish","tests","passes","ends","state","stay","timed","confirm","same","verify","running","again","refuses","because","cleaned","stop","tell"]
hash: "11662b410f8b543b61deca5a74e3566b31e906b2a98db721521dd1890c1165ea"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rework NAME 'Pick up from status.md and finish the tests'","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"timeout","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"--timeout","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"hev dispatch run","chunkId":"failure-modes#the-run-times-out"}]
sources: [{"chunkId":"failure-modes#the-run-times-out","url":"/docs/failure-modes#the-run-times-out","anchor":"the-run-times-out"}]
---

The supervisor enforces the order's timeout (120 minutes by default), ending the harness and everything it started while keeping the worktree and branch; continue with rework, and set a longer timeout for longer work.
