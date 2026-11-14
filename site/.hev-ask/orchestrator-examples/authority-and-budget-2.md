---
id: "orchestrator-examples#authority-and-budget-2"
title: "Orchestrator examples"
heading: "Authority and budget"
group: "Use"
order: 214
url: "/docs/orchestrator-examples#authority-and-budget-2"
anchor: "authority-and-budget-2"
terms: ["authority","budget","gauntlet","orchestrator","gets","rework","loop","itself","validate","pending","validations","notify","dispatch","cannot","start","other","work","scoped","small","daily","plus","three","lifetime","roughly","dozen","attempts","builder","time","runs","concurrent","restarts","validation","left","reach","there","scope","name","runsperday","allows","four"]
hash: "719ea1d0cf888a99cbd8e905c84493d0334eb379b56ba2aa460edba39bcbf840"
mode: "agent-primary"
facts: [{"kind":"code","literal":"rework","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"validate","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"say","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"notify","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"dispatch","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"runs_per_day: 4","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"max_lifetime: 3d","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"concurrent_runs: 1","chunkId":"orchestrator-examples#authority-and-budget-2"}]
sources: [{"chunkId":"orchestrator-examples#authority-and-budget-2","url":"/docs/orchestrator-examples#authority-and-budget-2","anchor":"authority-and-budget-2"}]
---

The Gauntlet orchestrator gets rework (the loop itself), validate for pending validations, and say and notify, but no dispatch, so it cannot start other work and is scoped to one run. A small daily budget plus a three-day lifetime cap it at roughly a dozen attempts, with one builder at a time.
