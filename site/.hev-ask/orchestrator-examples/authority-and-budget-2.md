---
id: "orchestrator-examples#authority-and-budget-2"
title: "Orchestrator examples"
heading: "Authority and budget"
group: "Use"
order: 224
url: "/docs/orchestrator-examples#authority-and-budget-2"
anchor: "authority-and-budget-2"
terms: ["authority","budget","gauntlet","orchestrator","rework","validate","notify","dispatch","scope","single","name","daily","three","lifetime","bound","roughly","twelve","attempts","regardless","verdicts","builder","time","runs","concurrent","loop","restarts","validation","left","pending","reach","there","start","other","work","runsperday","allows","four","reworks","validations","maxlifetime"]
hash: "719ea1d0cf888a99cbd8e905c84493d0334eb379b56ba2aa460edba39bcbf840"
mode: "agent-primary"
facts: [{"kind":"code","literal":"rework","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"validate","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"say","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"notify","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"dispatch","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"runs_per_day: 4","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"max_lifetime: 3d","chunkId":"orchestrator-examples#authority-and-budget-2"},{"kind":"code","literal":"concurrent_runs: 1","chunkId":"orchestrator-examples#authority-and-budget-2"}]
sources: [{"chunkId":"orchestrator-examples#authority-and-budget-2","url":"/docs/orchestrator-examples#authority-and-budget-2","anchor":"authority-and-budget-2"}]
---

The Gauntlet orchestrator has rework, validate, say and notify but no dispatch, and its scope is a single run name. Its daily run cap and three-day lifetime bound it to roughly twelve attempts regardless of verdicts, with one builder at a time.
