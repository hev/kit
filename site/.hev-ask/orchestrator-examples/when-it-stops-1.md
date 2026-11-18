---
id: "orchestrator-examples#when-it-stops-1"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 249
url: "/docs/orchestrator-examples#when-it-stops-1"
anchor: "when-it-stops-1"
terms: ["stops","release","coordinator","round","records","goal","after","fourteen","lifetime","reported","budget","exhaustion","rather","success","periodic","rotation","starts","fresh","lineage","short","summary","checkpoint","does","grow","long","train","json","rotate","writes","every","merged","green","notes","current","also","maxlifetime","running","being","rounds","rotateafter"]
hash: "554ccfb9306c81c2c63991a322f5a22fc4ad4215950468d76dc6cea70dc8b8bc"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal-met.json","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"max_lifetime: 14d","chunkId":"orchestrator-examples#when-it-stops-1"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrator-examples#when-it-stops-1"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops-1","url":"/docs/orchestrator-examples#when-it-stops-1","anchor":"when-it-stops-1"}]
---

The release coordinator stops when a round records the goal as met, or after its fourteen-day lifetime, which is reported as budget exhaustion rather than success. A periodic rotation starts a fresh lineage from a short summary so the checkpoint does not grow over a long train.
