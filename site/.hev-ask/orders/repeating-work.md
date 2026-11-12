---
id: "orders#repeating-work"
title: "Writing orders"
heading: "Repeating work"
group: "Use"
order: 217
url: "/docs/orders#repeating-work"
anchor: "repeating-work"
terms: ["repeating","work","setting","every","repeats","order","same","worktree","fresh","harness","session","round","until","provable","stop","condition","maximum","lifetime","bounds","both","require","rounds","never","overlap","keep","host","slot","tell","worker","evidence","ends","loop","notes","leave","repeat","prove","maxlifetime","bound","whole","default"]
hash: "654749f52c1c3faffd99e770d0ab29e51d96c0601d547a90855037f3e7f941af"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m","chunkId":"orders#repeating-work"},{"kind":"code","literal":"until","chunkId":"orders#repeating-work"},{"kind":"code","literal":"max_lifetime: 24h","chunkId":"orders#repeating-work"},{"kind":"code","literal":"every","chunkId":"orders#repeating-work"}]
sources: [{"chunkId":"orders#repeating-work","url":"/docs/orders#repeating-work","anchor":"repeating-work"}]
---

Setting every repeats an order in the same worktree with a fresh harness session each round, with until as a provable stop condition and a maximum lifetime that bounds the run. Both require every. Rounds never overlap and keep one host slot, so tell the worker what evidence ends the loop and what notes to leave.
