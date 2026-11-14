---
id: "orders#repeating-work"
title: "Writing orders"
heading: "Repeating work"
group: "Use"
order: 261
url: "/docs/orders#repeating-work"
anchor: "repeating-work"
terms: ["repeating","work","orders","repeat","interval","same","worktree","fresh","harness","session","round","optionally","until","provable","condition","bounded","lifetime","default","rounds","never","overlap","hold","host","slot","while","waiting","tell","worker","evidence","satisfies","notes","leave","every","order","prove","maxlifetime","bound","whole","both","fields"]
hash: "654749f52c1c3faffd99e770d0ab29e51d96c0601d547a90855037f3e7f941af"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m","chunkId":"orders#repeating-work"},{"kind":"code","literal":"until","chunkId":"orders#repeating-work"},{"kind":"code","literal":"max_lifetime: 24h","chunkId":"orders#repeating-work"},{"kind":"code","literal":"every","chunkId":"orders#repeating-work"}]
sources: [{"chunkId":"orders#repeating-work","url":"/docs/orders#repeating-work","anchor":"repeating-work"}]
---

Orders can repeat on an interval in the same worktree with a fresh harness session each round, optionally until a provable condition is met and bounded by a max lifetime (24h by default). Rounds never overlap and hold a host slot while waiting; tell the worker what evidence satisfies the condition and what notes to leave.
