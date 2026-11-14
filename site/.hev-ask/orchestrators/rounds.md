---
id: "orchestrators#rounds"
title: "Orchestrators"
heading: "Rounds"
group: "Use"
order: 248
url: "/docs/orchestrators#rounds"
anchor: "rounds"
terms: ["rounds","orchestrators","work","rather","long","session","code","gathers","spec","checkpoint","inbox","evidence","round","skipped","nothing","changed","otherwise","fresh","agent","acts","only","through","authorized","verbs","under","admission","scope","budget","must","write","fails","lineage","rotation","restarts","summary","checkpoints","drift","rotate","after","lived"]
hash: "013f67c223c1efabd0e1b855d2cc2c094f5460badd2c83322d452519d4c1e908"
mode: "agent-primary"
facts: [{"kind":"code","literal":"checkpoint.md","chunkId":"orchestrators#rounds"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrators#rounds"}]
sources: [{"chunkId":"orchestrators#rounds","url":"/docs/orchestrators#rounds","anchor":"rounds"}]
---

Orchestrators work in rounds rather than one long session: code gathers spec, checkpoint, inbox and run evidence; the round is skipped if nothing changed; otherwise a fresh agent session acts only through its authorized verbs under admission's scope and budget; and it must write a checkpoint or the round fails. Lineage rotation restarts from a summary when checkpoints drift.
