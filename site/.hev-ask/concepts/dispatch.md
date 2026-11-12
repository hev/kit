---
id: "concepts#dispatch"
title: "Concepts"
heading: "Dispatch"
group: "Use"
order: 48
url: "/docs/concepts#dispatch"
anchor: "dispatch"
terms: ["dispatch","dispatching","sends","order","host","checks","capacity","fetches","origin","creates","dedicated","branch","starts","supervisor","full","queues","first","name","base","target","waits","queue","slot","frees"]
hash: "967518bc2fcb76dc643f615bf51e544bec381e00a93e4967701e4428bfe29847"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ORDER.md","chunkId":"concepts#dispatch"},{"kind":"code","literal":"origin","chunkId":"concepts#dispatch"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"concepts#dispatch"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"concepts#dispatch"}]
sources: [{"chunkId":"concepts#dispatch","url":"/docs/concepts#dispatch","anchor":"dispatch"}]
---

Dispatching sends an order to its host, which checks the order and capacity, fetches origin, creates a dedicated branch and starts a supervisor. A full host queues the order first in, first out.
