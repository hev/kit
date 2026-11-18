---
id: "orchestrators#rounds"
title: "Orchestrators"
heading: "Rounds"
group: "Use"
order: 258
url: "/docs/orchestrators#rounds"
anchor: "rounds"
terms: ["rounds","long","lived","orchestrator","works","rather","growing","session","code","gathers","spec","checkpoint","inbox","evidence","round","skipped","nothing","changed","otherwise","fresh","agent","acts","within","authority","must","write","starts","empty","context","rotation","lineage","summary","checkpoints","drift","rotate","after","never","ever","gather","model"]
hash: "013f67c223c1efabd0e1b855d2cc2c094f5460badd2c83322d452519d4c1e908"
mode: "agent-primary"
facts: [{"kind":"code","literal":"checkpoint.md","chunkId":"orchestrators#rounds"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrators#rounds"}]
sources: [{"chunkId":"orchestrators#rounds","url":"/docs/orchestrators#rounds","anchor":"rounds"}]
---

A long-lived orchestrator works in rounds rather than one growing session: code gathers the spec, checkpoint, inbox and run evidence; the round is skipped if nothing changed; otherwise a fresh agent session acts within its authority and must write a checkpoint. Each round starts with empty context, and rotation starts a new lineage from a summary when checkpoints drift.
