---
id: "failure-modes#the-run-times-out"
title: "Failure modes"
heading: "The run times out"
group: "Operate"
order: 116
url: "/docs/failure-modes#the-run-times-out"
anchor: "the-run-times-out"
terms: ["times","supervisor","enforces","order","timeout","minutes","default","ending","harness","children","marking","worktree","branch","stay","rework","continue","status","notes","dispatch","name","pick","finish","tests","passes","ends","everything","started","state","timed","confirm","same","verify","running","again","refuses","because","cleaned","stop","tell","docs"]
hash: "11662b410f8b543b61deca5a74e3566b31e906b2a98db721521dd1890c1165ea"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rework NAME 'Pick up from status.md and finish the tests'","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"timeout","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"--timeout","chunkId":"failure-modes#the-run-times-out"},{"kind":"code","literal":"hev dispatch run","chunkId":"failure-modes#the-run-times-out"}]
sources: [{"chunkId":"failure-modes#the-run-times-out","url":"/docs/failure-modes#the-run-times-out","anchor":"the-run-times-out"}]
---

The supervisor enforces the order's timeout, 120 minutes by default, ending the harness and its children and marking the run timeout. The worktree and branch stay, and rework can continue from the status notes.
