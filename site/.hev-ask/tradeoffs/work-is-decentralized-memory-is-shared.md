---
id: "tradeoffs#work-is-decentralized-memory-is-shared"
title: "Tradeoffs"
heading: "Work is decentralized, memory is shared"
group: "Reference"
order: 393
url: "/docs/tradeoffs#work-is-decentralized-memory-is-shared"
anchor: "work-is-decentralized-memory-is-shared"
terms: ["work","decentralized","memory","shared","dispatch","central","scheduler","sender","asks","host","slot","full","order","waits","first","queue","priorities","person","limits","moving","waiting","capacity","number","config","deliberate","exception","default","traces","board","files","machine","team","wanting","archive","adds","layer","paid","cost","service","someone"]
hash: "7cbb951087b793bf2fa12bd6739493e3745704884a6f8352c35d25bda4517e92"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch run","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"},{"kind":"code","literal":"cap","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"},{"kind":"code","literal":"hev up","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"}]
sources: [{"chunkId":"tradeoffs#work-is-decentralized-memory-is-shared","url":"/docs/tradeoffs#work-is-decentralized-memory-is-shared","anchor":"work-is-decentralized-memory-is-shared"}]
---

Dispatch has no central scheduler: a sender asks a host for a slot and, when full, the order waits in that host's own first-in-first-out queue, with no priorities, per-person limits or moving a waiting order, and capacity is a number in each sender's config. Memory is the deliberate exception: by default traces and the board are files on each machine, and a team wanting one shared archive adds a hev layer key, which is how hev kit is paid for. The cost is that a shared archive is a service someone runs, and a host without it can still dispatch but cannot share what it learned.
