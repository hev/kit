---
id: "dispatch#states"
title: "Dispatch"
heading: "States"
group: "Use"
order: 198
url: "/docs/dispatch#states"
anchor: "states"
terms: ["states","running","done","harness","exit","proof","acceptance","failed","nonzero","stop","recorded","admission","timeout","died","needs","review","same","item","validation","twice","closed","settled","hand","terminal","launch","unconfirmed","windows","close","show","clean","report","state","meaning","supervisor","alive","exited","zero","signal","records","code"]
hash: "7dece3093f2660ad88bf82f6c665c530a705f49e20791ab3f9349250e32efc18"
mode: "agent-primary"
facts: [{"kind":"code","literal":"running","chunkId":"dispatch#states"},{"kind":"code","literal":"done","chunkId":"dispatch#states"},{"kind":"code","literal":"failed","chunkId":"dispatch#states"},{"kind":"code","literal":"timeout","chunkId":"dispatch#states"},{"kind":"code","literal":"died","chunkId":"dispatch#states"},{"kind":"code","literal":"needs-review","chunkId":"dispatch#states"},{"kind":"code","literal":"closed","chunkId":"dispatch#states"},{"kind":"code","literal":"close","chunkId":"dispatch#states"},{"kind":"code","literal":"launch-unconfirmed","chunkId":"dispatch#states"},{"kind":"code","literal":"ls","chunkId":"dispatch#states"},{"kind":"code","literal":"show","chunkId":"dispatch#states"},{"kind":"code","literal":"clean","chunkId":"dispatch#states"},{"kind":"code","literal":"report.md","chunkId":"dispatch#states"}]
sources: [{"chunkId":"dispatch#states","url":"/docs/dispatch#states","anchor":"states"}]
---

Run states: running, done (harness exit 0, not proof acceptance was met), failed (nonzero, stop recorded as 143, or failed admission), timeout, died, needs-review (same Acceptance item failed validation twice), closed (settled by hand, terminal), and launch-unconfirmed on Windows.
