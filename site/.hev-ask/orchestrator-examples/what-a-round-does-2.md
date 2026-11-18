---
id: "orchestrator-examples#what-a-round-does-2"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 244
url: "/docs/orchestrator-examples#what-a-round-does-2"
anchor: "what-a-round-does-2"
terms: ["round","does","gauntlet","loop","starts","validator","automatically","builder","finishes","failing","verdict","triggers","rework","appends","failed","items","order","restarts","same","branch","followed","validation","again","pass","makes","record","goal","tell","done","json","lands","evidence","changes","next","calls","model","fail","sends","runs","writes"]
hash: "c1dd1a475ba1d6732c2a89d5964bb86e3874c05bf3fac991d38d4d3cdc85fa9e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"done","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"qa","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"rework --from-validation","chunkId":"orchestrator-examples#what-a-round-does-2"},{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#what-a-round-does-2"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does-2","url":"/docs/orchestrator-examples#what-a-round-does-2","anchor":"what-a-round-does-2"}]
---

In the Gauntlet loop kit starts the validator automatically when the builder finishes. A failing verdict triggers a rework that appends the failed items to the order and restarts the builder on the same branch, followed by validation again; a pass makes the round record the goal as met and tell you.
