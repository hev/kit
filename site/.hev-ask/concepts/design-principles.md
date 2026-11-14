---
id: "concepts#design-principles"
title: "Concepts"
heading: "Design principles"
group: "Use"
order: 102
url: "/docs/concepts#design-principles"
anchor: "design-principles"
terms: ["design","principles","binary","acting","daemon","supervisor","remote","endpoint","config","state","plain","files","host","there","central","scheduler","only","fifo","queues","come","first","shared","archive","teams","receiving","decides","policy","every","traced","tagged","order","name","these","visible","code","explain","most","behavior","dispatch","call"]
hash: "0c83f8043120e5147d0148bc4d5ec5869239317022a4bebb51005f688c11c567"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"concepts#design-principles"},{"kind":"code","literal":"cat","chunkId":"concepts#design-principles"}]
sources: [{"chunkId":"concepts#design-principles","url":"/docs/concepts#design-principles","anchor":"design-principles"}]
---

hev is one Go binary acting as CLI, daemon, supervisor and remote endpoint; config and run state are plain files per host; there is no central scheduler, only SSH and per-host FIFO queues; files come first, with a shared archive for teams; the receiving host decides policy; and every run is traced and tagged with its order name.
