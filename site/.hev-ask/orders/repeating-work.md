---
id: "orders#repeating-work"
title: "Writing orders"
heading: "Repeating work"
group: "Use"
order: 271
url: "/docs/orders#repeating-work"
anchor: "repeating-work"
terms: ["repeating","work","order","repeat","interval","same","worktree","fresh","harness","session","round","provable","stop","condition","maximum","lifetime","defaults","rounds","never","overlap","hold","host","slot","while","waiting","tell","worker","evidence","satisfies","notes","leave","next","every","until","prove","maxlifetime","bound","whole","default","both"]
hash: "654749f52c1c3faffd99e770d0ab29e51d96c0601d547a90855037f3e7f941af"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m","chunkId":"orders#repeating-work"},{"kind":"code","literal":"until","chunkId":"orders#repeating-work"},{"kind":"code","literal":"max_lifetime: 24h","chunkId":"orders#repeating-work"},{"kind":"code","literal":"every","chunkId":"orders#repeating-work"}]
sources: [{"chunkId":"orders#repeating-work","url":"/docs/orders#repeating-work","anchor":"repeating-work"}]
---

An order can repeat at an interval in the same worktree with a fresh harness session each round, with a provable stop condition and a maximum lifetime that defaults to a day. Rounds never overlap and hold one host slot while waiting; tell the worker what evidence satisfies the condition and what notes to leave for the next round.
