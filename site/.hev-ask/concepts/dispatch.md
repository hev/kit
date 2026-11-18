---
id: "concepts#dispatch"
title: "Concepts"
heading: "Dispatch"
group: "Use"
order: 104
url: "/docs/concepts#dispatch"
anchor: "dispatch"
terms: ["dispatch","dispatching","sends","order","host","target","checks","capacity","fetches","origin","creates","fresh","branch","starts","supervisor","full","waits","first","queue","until","slot","frees","name","base"]
hash: "967518bc2fcb76dc643f615bf51e544bec381e00a93e4967701e4428bfe29847"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ORDER.md","chunkId":"concepts#dispatch"},{"kind":"code","literal":"origin","chunkId":"concepts#dispatch"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"concepts#dispatch"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"concepts#dispatch"}]
sources: [{"chunkId":"concepts#dispatch","url":"/docs/concepts#dispatch","anchor":"dispatch"}]
---

Dispatching sends an order to its host, where the target checks the order and its own capacity, fetches origin, creates a fresh branch and starts a supervisor. If the host is full, the order waits in the host's first-in, first-out queue until a slot frees.
