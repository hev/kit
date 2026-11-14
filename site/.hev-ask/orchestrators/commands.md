---
id: "orchestrators#commands"
title: "Orchestrators"
heading: "Commands"
group: "Use"
order: 243
url: "/docs/orchestrators#commands"
anchor: "commands"
terms: ["commands","orchestrate","command","family","apply","spec","start","runs","list","show","read","round","logs","pause","resume","wake","stop","hand","another","owner","state","lives","orchestrator","directory","host","checkpoint","inbox","rounds","writing","waking","triggers","early","never","overlaps","running","dispatch","orch","name","release","train"]
hash: "25ec1c1bcaeb7fdcb26fb1530822df8a5c21fffe0e6ee624d718670fa47d7ef2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/dispatch/orch-NAME/","chunkId":"orchestrators#commands"},{"kind":"code","literal":"spec.md","chunkId":"orchestrators#commands"},{"kind":"code","literal":"checkpoint.md","chunkId":"orchestrators#commands"},{"kind":"code","literal":"inbox.md","chunkId":"orchestrators#commands"},{"kind":"code","literal":"rounds/NNN/","chunkId":"orchestrators#commands"},{"kind":"code","literal":"wake","chunkId":"orchestrators#commands"}]
sources: [{"chunkId":"orchestrators#commands","url":"/docs/orchestrators#commands","anchor":"commands"}]
---

The orchestrate command family: dry-run and apply a spec, start an ad-hoc one over your own runs, list, show, read round logs, pause, resume, wake, stop and hand off to another owner. State lives in a per-orchestrator directory on the host that runs it (spec, checkpoint, inbox, rounds); writing to the inbox and waking triggers an early round that never overlaps a running one.
