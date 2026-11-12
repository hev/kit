---
id: "dispatch#states"
title: "Dispatch"
heading: "States"
group: "Use"
order: 94
url: "/docs/dispatch#states"
anchor: "states"
terms: ["states","running","done","harness","exited","cleanly","proof","acceptance","failed","including","stops","exit","code","admission","timeout","died","supervisor","vanished","needs","review","same","item","validation","twice","launch","unconfirmed","windows","show","clean","report","state","meaning","alive","zero","stop","signal","records","killed","live","final"]
hash: "b4ae5b2d8a7dbd2a97551185330f929e734de71a691be0a84c7e406ff1e6d5fd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"running","chunkId":"dispatch#states"},{"kind":"code","literal":"done","chunkId":"dispatch#states"},{"kind":"code","literal":"failed","chunkId":"dispatch#states"},{"kind":"code","literal":"timeout","chunkId":"dispatch#states"},{"kind":"code","literal":"died","chunkId":"dispatch#states"},{"kind":"code","literal":"needs-review","chunkId":"dispatch#states"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"dispatch#states"},{"kind":"code","literal":"ls","chunkId":"dispatch#states"},{"kind":"code","literal":"show","chunkId":"dispatch#states"},{"kind":"code","literal":"clean","chunkId":"dispatch#states"},{"kind":"code","literal":"report.md","chunkId":"dispatch#states"}]
sources: [{"chunkId":"dispatch#states","url":"/docs/dispatch#states","anchor":"states"}]
---

Run states are running, done (harness exited cleanly, not proof acceptance was met), failed (including stops with exit code 143 and failed admission), timeout, died (supervisor vanished), needs-review (same item failed validation twice) and launch-unconfirmed on Windows.
