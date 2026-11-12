---
id: "orders#repeating-work"
title: "Writing orders"
heading: "Repeating work"
group: "Use"
order: 211
url: "/docs/orders#repeating-work"
anchor: "repeating-work"
terms: ["repeating","work","cadence","repeat","order","same","worktree","fresh","sessions","plus","provable","stop","condition","lifetime","bound","rounds","never","overlap","hold","slot","should","tell","worker","evidence","satisfies","notes","leave","every","until","harness","session","round","prove","maxlifetime","whole","default","both","fields","require","keep"]
hash: "654749f52c1c3faffd99e770d0ab29e51d96c0601d547a90855037f3e7f941af"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m","chunkId":"orders#repeating-work"},{"kind":"code","literal":"until","chunkId":"orders#repeating-work"},{"kind":"code","literal":"max_lifetime: 24h","chunkId":"orders#repeating-work"},{"kind":"code","literal":"every","chunkId":"orders#repeating-work"}]
sources: [{"chunkId":"orders#repeating-work","url":"/docs/orders#repeating-work","anchor":"repeating-work"}]
---

Set a cadence to repeat an order in the same worktree with fresh sessions, plus a provable stop condition and a lifetime bound; rounds never overlap and hold a slot, and you should tell the worker what evidence satisfies the condition and what notes to leave.
