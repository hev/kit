---
id: "orchestrators#rounds"
title: "Orchestrators"
heading: "Rounds"
group: "Use"
order: 198
url: "/docs/orchestrators#rounds"
anchor: "rounds"
terms: ["rounds","orchestrators","work","code","gathers","spec","checkpoint","inbox","evidence","round","skipped","nothing","changed","otherwise","fresh","session","acts","only","through","allowed","verbs","admission","enforcing","scope","budget","must","write","fails","lineage","rotation","starts","summary","checkpoints","drift","rotate","after","long","lived","orchestrator","never"]
hash: "013f67c223c1efabd0e1b855d2cc2c094f5460badd2c83322d452519d4c1e908"
mode: "agent-primary"
facts: [{"kind":"code","literal":"checkpoint.md","chunkId":"orchestrators#rounds"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrators#rounds"}]
sources: [{"chunkId":"orchestrators#rounds","url":"/docs/orchestrators#rounds","anchor":"rounds"}]
---

Orchestrators work in rounds: code gathers spec, checkpoint, inbox and run evidence; the round is skipped if nothing changed; otherwise a fresh session acts only through its allowed verbs with admission enforcing scope and budget; and it must write a checkpoint or the round fails. Lineage rotation starts fresh from a summary when checkpoints drift.
