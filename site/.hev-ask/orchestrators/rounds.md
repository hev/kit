---
id: "orchestrators#rounds"
title: "Orchestrators"
heading: "Rounds"
group: "Use"
order: 321
url: "/docs/orchestrators#rounds"
anchor: "rounds"
terms: ["rounds","orchestrators","work","never","growing","session","code","gathers","state","skips","model","nothing","changed","fresh","acts","only","authority","verbs","admission","rejects","scope","budget","actions","round","must","write","checkpoint","covering","goal","status","open","threads","waiting","next","action","rotate","after","long","lived","orchestrator"]
hash: "013f67c223c1efabd0e1b855d2cc2c094f5460badd2c83322d452519d4c1e908"
mode: "agent-primary"
facts: [{"kind":"code","literal":"checkpoint.md","chunkId":"orchestrators#rounds"},{"kind":"code","literal":"rotate_after","chunkId":"orchestrators#rounds"}]
sources: [{"chunkId":"orchestrators#rounds","url":"/docs/orchestrators#rounds","anchor":"rounds"}]
---

Orchestrators work in rounds, never one growing session: code gathers state, skips the model if nothing changed, a fresh session acts only with its authority verbs (admission rejects out-of-scope or over-budget actions), and the round must write a checkpoint covering goal status, open threads, waiting-on and next action.
