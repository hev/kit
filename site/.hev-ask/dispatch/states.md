---
id: "dispatch#states"
title: "Dispatch"
heading: "States"
group: "Use"
order: 156
url: "/docs/dispatch#states"
anchor: "states"
terms: ["states","running","done","failed","timeout","died","needs","review","windows","only","launch","unconfirmed","means","harness","exited","cleanly","acceptance","read","report","stop","recorded","exit","code","show","clean","state","meaning","supervisor","alive","zero","signal","admission","records","killed","live","final","result","same","item","browser"]
hash: "b4ae5b2d8a7dbd2a97551185330f929e734de71a691be0a84c7e406ff1e6d5fd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"running","chunkId":"dispatch#states"},{"kind":"code","literal":"done","chunkId":"dispatch#states"},{"kind":"code","literal":"failed","chunkId":"dispatch#states"},{"kind":"code","literal":"timeout","chunkId":"dispatch#states"},{"kind":"code","literal":"died","chunkId":"dispatch#states"},{"kind":"code","literal":"needs-review","chunkId":"dispatch#states"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"dispatch#states"},{"kind":"code","literal":"ls","chunkId":"dispatch#states"},{"kind":"code","literal":"show","chunkId":"dispatch#states"},{"kind":"code","literal":"clean","chunkId":"dispatch#states"},{"kind":"code","literal":"report.md","chunkId":"dispatch#states"}]
sources: [{"chunkId":"dispatch#states","url":"/docs/dispatch#states","anchor":"states"}]
---

The run states: running, done, failed, timeout, died, needs-review and a Windows-only launch-unconfirmed. Done means only that the harness exited cleanly, not that acceptance was met, so read the report and the PR; a stop is recorded as failed with exit code 143.
