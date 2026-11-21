---
id: "orchestrator-examples#when-it-stops-2"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 313
url: "/docs/orchestrator-examples#when-it-stops-2"
anchor: "when-it-stops-2"
terms: ["stops","gauntlet","loop","work","wins","items","pass","same","item","fails","twice","needs","review","verdict","based","rework","refused","plain","note","would","reset","count","admission","doesn","prevent","leave","guarantee","budget","runs","daily","waits","till","midnight","days","goal","json","reply","whichever","comes","first"]
hash: "238f53e1f7516b92b62bb2f206e848cf169b27e43d229f4643a1dc8f3eb07af2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#when-it-stops-2"},{"kind":"code","literal":"needs-review","chunkId":"orchestrator-examples#when-it-stops-2"},{"kind":"code","literal":"rework","chunkId":"orchestrator-examples#when-it-stops-2"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#when-it-stops-2"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops-2","url":"/docs/orchestrator-examples#when-it-stops-2","anchor":"when-it-stops-2"}]
---

The Gauntlet loop stops when the work wins (all items pass), when the same item fails twice (needs-review and verdict-based rework refused; a plain rework with a note would reset the count, which admission doesn't prevent, so leave rework out for a guarantee), or when budget runs out (daily cap waits till midnight UTC; 3 days stops).
