---
id: "orchestrators#commands"
title: "Orchestrators"
heading: "Commands"
group: "Use"
order: 253
url: "/docs/orchestrators#commands"
anchor: "commands"
terms: ["commands","lists","orchestrator","lifecycle","apply","spec","start","list","show","read","round","logs","pause","resume","wake","stop","hand","state","checkpoint","inbox","rounds","lives","directory","host","never","overlaps","running","runs","launches","join","rooted","trace","only","inherit","dispatch","orch","name","orchestrate","release","train"]
hash: "5a4a2cdb576095583aff1769b548f1c2c382093c85f21233304ec5eb58a4696f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/dispatch/orch-NAME/","chunkId":"orchestrators#commands"},{"kind":"code","literal":"spec.md","chunkId":"orchestrators#commands"},{"kind":"code","literal":"checkpoint.md","chunkId":"orchestrators#commands"},{"kind":"code","literal":"inbox.md","chunkId":"orchestrators#commands"},{"kind":"code","literal":"rounds/NNN/","chunkId":"orchestrators#commands"},{"kind":"code","literal":"wake","chunkId":"orchestrators#commands"}]
sources: [{"chunkId":"orchestrators#commands","url":"/docs/orchestrators#commands","anchor":"commands"}]
---

Lists the orchestrator lifecycle commands: dry-run and apply a spec, start an ad-hoc one, list, show, read round logs, pause, resume, wake, stop and hand off. State (spec, checkpoint, inbox, rounds) lives in a per-orchestrator directory on its host, a wake never overlaps a running round, and runs it launches join an ask-rooted trace only if the round has one to inherit.
