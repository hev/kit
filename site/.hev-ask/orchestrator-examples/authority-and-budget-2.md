---
id: "orchestrator-examples#authority-and-budget-2"
title: "Orchestrator examples"
heading: "Authority and budget"
group: "Use"
order: 170
url: "/docs/orchestrator-examples#authority-and-budget-2"
anchor: "authority-and-budget-2"
terms: ["authority","budget","gauntlet","loop","rework","validate","notify","dispatch","scoped","daily","four","three","lifetime","bound","about","twelve","attempts","builder","time","runs","concurrent","restarts","validation","left","pending","reach","there","start","other","work","scope","name","runsperday","allows","reworks","validations","maxlifetime","ends","after","days"]
hash: "719ea1d0cf888a99cbd8e905c84493d0334eb379b56ba2aa460edba39bcbf840"
mode: "agent-primary"
facts: [{"kind":"code","literal":"rework","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"validate","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"say","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"notify","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"dispatch","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"runs_per_day: 4","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"max_lifetime: 3d","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"concurrent_runs: 1","chunkId":"orchestrator-examples#authority-and-budget-2"}]
sources: [{"chunkId":"orchestrator-examples#authority-and-budget-2","url":"/docs/orchestrator-examples#authority-and-budget-2","anchor":"authority-and-budget-2"}]
---

The Gauntlet loop can rework, validate, say and notify but not dispatch, and is scoped to one run. A daily cap of four and a three day lifetime bound it to about twelve attempts, with one builder at a time.
