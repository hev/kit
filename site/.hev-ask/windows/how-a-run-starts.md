---
id: "windows#how-a-run-starts"
title: "Windows"
heading: "How a run starts"
group: "Reference"
order: 318
url: "/docs/windows#how-a-run-starts"
anchor: "how-a-run-starts"
terms: ["starts","dispatch","supervisor","detached","process","falling","back","shot","scheduled","task","forbids","breakaway","runs","harness","kill","close","object","stop","timeout","ends","everything","started","slots","counted","plus","creation","time","unconfirmed","launch","shows","marked","failed","after","seconds","name","show","clean","breaks","away","session"]
hash: "ecf54856f2d6e3a95a798ffa565595ae86a899b199e231dc1f4ab355c1366fa0"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch rpc","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"hev-dispatch-NAME","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"ls","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"show","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"clean","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"failed","chunkId":"windows#how-a-run-starts"}]
sources: [{"chunkId":"windows#how-a-run-starts","url":"/docs/windows#how-a-run-starts","anchor":"how-a-run-starts"}]
---

dispatch rpc starts the supervisor as a detached process, falling back to a one-shot scheduled task if SSH forbids breakaway. The supervisor runs the harness in a kill-on-close job object, so stop or timeout ends everything it started. Slots are counted from supervisor PID plus creation time, and an unconfirmed launch shows as launch-unconfirmed and is marked failed after 30 seconds.
