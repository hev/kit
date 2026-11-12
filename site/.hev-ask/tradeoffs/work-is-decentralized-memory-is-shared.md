---
id: "tradeoffs#work-is-decentralized-memory-is-shared"
title: "Tradeoffs"
heading: "Work is decentralized, memory is shared"
group: "Reference"
order: 300
url: "/docs/tradeoffs#work-is-decentralized-memory-is-shared"
anchor: "work-is-decentralized-memory-is-shared"
terms: ["work","decentralized","memory","shared","dispatch","central","scheduler","sender","asks","host","slot","full","queues","order","first","cost","simple","scheduling","priorities","person","limits","moving","queued","picks","most","free","slots","only","moment","capacity","config","value","deliberate","exception","traces","board","hosts","report","server","waits"]
hash: "7cbb951087b793bf2fa12bd6739493e3745704884a6f8352c35d25bda4517e92"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch run","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"},{"kind":"code","literal":"cap","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"},{"kind":"code","literal":"hev up","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"}]
sources: [{"chunkId":"tradeoffs#work-is-decentralized-memory-is-shared","url":"/docs/tradeoffs#work-is-decentralized-memory-is-shared","anchor":"work-is-decentralized-memory-is-shared"}]
---

Dispatch has no central scheduler: a sender asks a host for a slot and a full host queues the order first in, first out. The cost is simple scheduling with no priorities, per-person limits or moving a queued order; run picks the host with most free slots only at that moment, and capacity is each sender's config value. Memory is the deliberate exception, with traces and the board shared.
