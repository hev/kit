---
id: "orchestrator-examples#when-it-stops-1"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 189
url: "/docs/orchestrator-examples#when-it-stops-1"
anchor: "when-it-stops-1"
terms: ["stops","release","coordinator","round","writes","goal","marker","after","lifetime","reported","budget","running","periodically","rotates","lineage","keep","checkpoints","short","json","rotate","every","train","merged","green","notes","current","also","maxlifetime","being","rounds","rotateafter","starts","summary","week","doesn","build","long","checkpoint"]
hash: "554ccfb9306c81c2c63991a322f5a22fc4ad4215950468d76dc6cea70dc8b8bc"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"max_lifetime: 14d","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrator-examples#when-it-stops-1"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops-1","url":"/docs/orchestrator-examples#when-it-stops-1","anchor":"when-it-stops-1"}]
---

The release coordinator stops when a round writes the goal marker or after its 14-day lifetime, which is reported as the budget running out, and periodically rotates to a new lineage to keep checkpoints short.
