---
id: "windows#how-a-run-starts"
title: "Windows"
heading: "How a run starts"
group: "Reference"
order: 304
url: "/docs/windows#how-a-run-starts"
anchor: "how-a-run-starts"
terms: ["starts","dispatch","supervisor","detached","process","breaking","away","shot","scheduled","task","breakaway","allowed","harness","runs","inside","object","stopping","timing","kills","every","started","slots","counted","creation","time","whose","start","cannot","confirmed","shows","launch","unconfirmed","marked","failed","after","seconds","running","name","show","clean"]
hash: "ecf54856f2d6e3a95a798ffa565595ae86a899b199e231dc1f4ab355c1366fa0"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch rpc","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"hev-dispatch-NAME","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"ls","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"show","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"clean","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"failed","chunkId":"windows#how-a-run-starts"}]
sources: [{"chunkId":"windows#how-a-run-starts","url":"/docs/windows#how-a-run-starts","anchor":"how-a-run-starts"}]
---

The dispatch RPC starts the supervisor as a detached process breaking away from the SSH job, or via a one-shot scheduled task if breakaway is not allowed. The harness runs inside a job object so stopping or timing out kills every process it started, slots are counted by supervisor PID and creation time, and a run whose start cannot be confirmed shows as launch-unconfirmed and is marked failed after 30 seconds if no supervisor is running.
