---
id: "windows#how-a-run-starts"
title: "Windows"
heading: "How a run starts"
group: "Reference"
order: 417
url: "/docs/windows#how-a-run-starts"
anchor: "how-a-run-starts"
terms: ["starts","windows","dispatched","supervisor","detached","process","breaks","away","session","falling","back","shot","scheduled","task","server","forbids","breakaway","places","harness","object","stopping","hitting","timeout","ends","everything","spawned","slot","occupancy","counted","plus","creation","time","reused","does","hold","whose","start","cannot","confirmed","shows"]
hash: "ecf54856f2d6e3a95a798ffa565595ae86a899b199e231dc1f4ab355c1366fa0"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch rpc","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"hev-dispatch-NAME","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"ls","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"show","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"clean","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"failed","chunkId":"windows#how-a-run-starts"}]
sources: [{"chunkId":"windows#how-a-run-starts","url":"/docs/windows#how-a-run-starts","anchor":"how-a-run-starts"}]
---

On Windows a dispatched run's supervisor starts as a detached process that breaks away from the SSH session's job, falling back to a one-shot scheduled task if the SSH server forbids breakaway. The supervisor places the harness in a job object so stopping the run or hitting its timeout ends the harness and everything it spawned. Slot occupancy is counted by supervisor process ID plus creation time so a reused ID does not hold a slot, and a run whose start cannot be confirmed shows as launch-unconfirmed, is marked failed after 30 seconds if no supervisor is running, and cannot be cleaned while unconfirmed.
