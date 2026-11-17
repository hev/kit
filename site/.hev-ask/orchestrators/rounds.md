---
id: "orchestrators#rounds"
title: "Orchestrators"
heading: "Rounds"
group: "Use"
order: 204
url: "/docs/orchestrators#rounds"
anchor: "rounds"
terms: ["rounds","long","lived","orchestrator","works","rather","growing","session","code","gathers","spec","last","checkpoint","inbox","state","round","skipped","nothing","changed","otherwise","fresh","agent","acts","only","authority","verbs","admission","enforcing","scope","budget","must","write","file","fails","previous","stands","rotate","setting","restarts","lineage"]
hash: "013f67c223c1efabd0e1b855d2cc2c094f5460badd2c83322d452519d4c1e908"
mode: "agent-primary"
facts: [{"kind":"code","literal":"checkpoint.md","chunkId":"orchestrators#rounds"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrators#rounds"}]
sources: [{"chunkId":"orchestrators#rounds","url":"/docs/orchestrators#rounds","anchor":"rounds"}]
---

A long-lived orchestrator works in rounds rather than one growing session. Code gathers the spec, last checkpoint, inbox and run state; the round is skipped if nothing changed; otherwise a fresh agent acts using only its authority verbs, with admission enforcing scope and budget. Each round must write a checkpoint file or it fails and the previous checkpoint stands, and a rotate setting restarts the lineage when checkpoints drift.
