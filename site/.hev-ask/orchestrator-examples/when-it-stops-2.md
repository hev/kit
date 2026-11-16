---
id: "orchestrator-examples#when-it-stops-2"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 250
url: "/docs/orchestrator-examples#when-it-stops-2"
anchor: "when-it-stops-2"
terms: ["stops","gauntlet","loop","whichever","comes","first","every","item","passes","same","fails","twice","goes","needs","review","daily","lifetime","budget","runs","plain","rework","would","reset","fail","count","admission","does","block","omit","authority","need","guaranteed","goal","json","reply","work","wins","round","writes","gate"]
hash: "238f53e1f7516b92b62bb2f206e848cf169b27e43d229f4643a1dc8f3eb07af2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#when-it-stops-2"},{"kind":"code","literal":"needs-review","chunkId":"orchestrator-examples#when-it-stops-2"},{"kind":"code","literal":"rework","chunkId":"orchestrator-examples#when-it-stops-2"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#when-it-stops-2"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops-2","url":"/docs/orchestrator-examples#when-it-stops-2","anchor":"when-it-stops-2"}]
---

The Gauntlet loop stops on whichever comes first: every item passes, the same item fails twice and the run goes to needs-review, or the daily or lifetime budget runs out. A plain rework would reset the fail count and admission does not block it, so omit rework authority if you need that guaranteed.
