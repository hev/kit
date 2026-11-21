---
id: "orchestrator-examples#what-a-round-does-2"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 307
url: "/docs/orchestrator-examples#what-a-round-does-2"
anchor: "what-a-round-does-2"
terms: ["round","does","gauntlet","loop","after","builder","finishes","starts","validation","verdict","lands","next","calls","model","fail","sends","rework","appending","failed","items","restarting","same","branch","pass","writes","goal","tells","done","json","validator","evidence","changes","appends","order","again","runs"]
hash: "c1dd1a475ba1d6732c2a89d5964bb86e3874c05bf3fac991d38d4d3cdc85fa9e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"done","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"qa","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"rework --from-validation","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#what-a-round-does-2"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does-2","url":"/docs/orchestrator-examples#what-a-round-does-2","anchor":"what-a-round-does-2"}]
---

A round of the Gauntlet loop: after the builder finishes, kit starts validation on its own; when a verdict lands the next round calls the model. On fail it sends rework from validation appending failed items and restarting the builder on the same branch; on pass it writes goal-met and tells you.
