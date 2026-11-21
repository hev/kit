---
id: "orders#repeating-work"
title: "Writing orders"
heading: "Repeating work"
group: "Use"
order: 334
url: "/docs/orders#repeating-work"
anchor: "repeating-work"
terms: ["repeating","work","every","repeats","order","same","worktree","fresh","sessions","round","until","gives","provable","stop","condition","maxlifetime","bounds","default","both","require","rounds","never","overlap","hold","host","slot","lifetime","repeat","harness","session","worker","prove","bound","whole","fields","keep","while","waiting","tell","evidence"]
hash: "654749f52c1c3faffd99e770d0ab29e51d96c0601d547a90855037f3e7f941af"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m","chunkId":"orders#repeating-work"},{"kind":"code","literal":"until","chunkId":"orders#repeating-work"},{"kind":"code","literal":"max_lifetime: 24h","chunkId":"orders#repeating-work"},{"kind":"code","literal":"every","chunkId":"orders#repeating-work"}]
sources: [{"chunkId":"orders#repeating-work","url":"/docs/orders#repeating-work","anchor":"repeating-work"}]
---

Repeating work: every repeats the order in the same worktree with fresh sessions each round; until gives a provable stop condition and maxlifetime bounds the run (default with every). Both require every; rounds never overlap and hold one host slot.
