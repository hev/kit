---
id: "orchestrator-examples#what-a-round-does-2"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 184
url: "/docs/orchestrator-examples#what-a-round-does-2"
anchor: "what-a-round-does-2"
terms: ["round","does","gauntlet","loop","validator","starts","automatically","builder","finishes","failing","verdict","next","sends","rework","validation","pass","writes","goal","marker","tells","done","json","lands","evidence","changes","calls","model","fail","appends","failed","items","order","again","same","branch","runs"]
hash: "c1dd1a475ba1d6732c2a89d5964bb86e3874c05bf3fac991d38d4d3cdc85fa9e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"done","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"qa","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"rework --from-validation","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#what-a-round-does-2"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does-2","url":"/docs/orchestrator-examples#what-a-round-does-2","anchor":"what-a-round-does-2"}]
---

In the Gauntlet loop, the validator starts automatically when the builder finishes; on a failing verdict the next round sends rework from validation, and on a pass it writes the goal marker and tells you.
