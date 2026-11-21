---
id: "orchestrator-examples#when-it-stops-1"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 312
url: "/docs/orchestrator-examples#when-it-stops-1"
anchor: "when-it-stops-1"
terms: ["stops","release","coordinator","round","writes","goal","merged","green","notes","current","after","lifetime","reported","budget","exhausted","rather","rotation","every","rounds","restarts","short","summary","avoid","long","checkpoints","json","rotate","train","also","maxlifetime","running","being","rotateafter","starts","lineage","week","doesn","build","checkpoint"]
hash: "554ccfb9306c81c2c63991a322f5a22fc4ad4215950468d76dc6cea70dc8b8bc"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"max_lifetime: 14d","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrator-examples#when-it-stops-1"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops-1","url":"/docs/orchestrator-examples#when-it-stops-1","anchor":"when-it-stops-1"}]
---

The release coordinator stops when a round writes goal-met (all PRs merged, CI green, notes current) or after a 14-day lifetime, which is reported as budget exhausted rather than goal met. A rotation every 30 rounds restarts from a short summary to avoid long checkpoints.
