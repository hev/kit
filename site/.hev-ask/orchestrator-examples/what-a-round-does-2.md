---
id: "orchestrator-examples#what-a-round-does-2"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 190
url: "/docs/orchestrator-examples#what-a-round-does-2"
anchor: "what-a-round-does-2"
terms: ["round","does","builder","finishes","starts","validator","itself","verdict","lands","next","calls","model","fail","sends","rework","validation","same","branch","pass","writes","goal","file","tells","done","json","evidence","changes","appends","failed","items","order","again","runs"]
hash: "c1dd1a475ba1d6732c2a89d5964bb86e3874c05bf3fac991d38d4d3cdc85fa9e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"done","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"qa","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"rework --from-validation","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#what-a-round-does-2"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does-2","url":"/docs/orchestrator-examples#what-a-round-does-2","anchor":"what-a-round-does-2"}]
---

The builder finishes and kit starts the validator itself. When a verdict lands the next round calls the model: on a fail it sends rework from the validation to the same branch, on a pass it writes the goal-met file and tells you.
