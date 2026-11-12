---
id: "orchestrator-examples#when-it-stops-1"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 195
url: "/docs/orchestrator-examples#when-it-stops-1"
anchor: "when-it-stops-1"
terms: ["stops","release","coordinator","round","writes","goal","file","after","lifetime","reported","budget","exhaustion","rather","success","every","rounds","starts","lineage","short","summary","checkpoint","stays","small","json","rotate","train","merged","green","notes","current","also","maxlifetime","running","being","rotateafter","week","doesn","build","long"]
hash: "554ccfb9306c81c2c63991a322f5a22fc4ad4215950468d76dc6cea70dc8b8bc"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"max_lifetime: 14d","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrator-examples#when-it-stops-1"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops-1","url":"/docs/orchestrator-examples#when-it-stops-1","anchor":"when-it-stops-1"}]
---

The release coordinator stops when a round writes the goal-met file, or after its 14 day lifetime, which is reported as budget exhaustion rather than success. Every 30 rounds it starts a new lineage from a short summary so the checkpoint stays small.
