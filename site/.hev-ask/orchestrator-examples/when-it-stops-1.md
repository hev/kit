---
id: "orchestrator-examples#when-it-stops-1"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 239
url: "/docs/orchestrator-examples#when-it-stops-1"
anchor: "when-it-stops-1"
terms: ["stops","release","coordinator","round","marks","goal","train","merged","green","notes","current","after","lifetime","reported","budget","exhaustion","rather","success","periodic","lineage","rotation","keeps","long","running","checkpoint","short","json","rotate","writes","every","also","maxlifetime","being","rounds","rotateafter","starts","summary","week","doesn","build"]
hash: "554ccfb9306c81c2c63991a322f5a22fc4ad4215950468d76dc6cea70dc8b8bc"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"max_lifetime: 14d","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrator-examples#when-it-stops-1"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops-1","url":"/docs/orchestrator-examples#when-it-stops-1","anchor":"when-it-stops-1"}]
---

The release coordinator stops when a round marks the goal met (all train PRs merged, CI green, notes current) or after its max lifetime, which is reported as budget exhaustion rather than success. Periodic lineage rotation keeps a long-running train's checkpoint short.
