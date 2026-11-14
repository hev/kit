---
id: "windows#how-a-run-starts"
title: "Windows"
heading: "How a run starts"
group: "Reference"
order: 366
url: "/docs/windows#how-a-run-starts"
anchor: "how-a-run-starts"
terms: ["starts","windows","dispatch","launches","supervisor","detached","process","breaking","away","shot","scheduled","task","breakaway","blocked","harness","runs","inside","object","stopping","timing","kills","whole","tree","slots","tracked","plus","creation","time","unconfirmed","marked","failed","after","seconds","exists","name","launch","show","clean","breaks","session"]
hash: "ecf54856f2d6e3a95a798ffa565595ae86a899b199e231dc1f4ab355c1366fa0"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch rpc","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"hev-dispatch-NAME","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"ls","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"show","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"clean","chunkId":"windows#how-a-run-starts"},{"kind":"code","literal":"failed","chunkId":"windows#how-a-run-starts"}]
sources: [{"chunkId":"windows#how-a-run-starts","url":"/docs/windows#how-a-run-starts","anchor":"how-a-run-starts"}]
---

On Windows the dispatch RPC launches the supervisor as a detached process breaking away from the SSH job, or via a one-shot scheduled task if breakaway is blocked. The harness runs inside a job object so stopping or timing out kills its whole process tree; slots are tracked by PID plus creation time, and unconfirmed launches are marked failed after 30 seconds if no supervisor exists.
