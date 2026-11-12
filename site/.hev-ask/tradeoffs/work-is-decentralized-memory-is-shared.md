---
id: "tradeoffs#work-is-decentralized-memory-is-shared"
title: "Tradeoffs"
heading: "Work is decentralized, memory is shared"
group: "Reference"
order: 298
url: "/docs/tradeoffs#work-is-decentralized-memory-is-shared"
anchor: "work-is-decentralized-memory-is-shared"
terms: ["work","decentralized","memory","shared","dispatch","central","scheduler","senders","hosts","slots","full","queue","orders","first","there","priorities","person","limits","moving","waiting","capacity","sender","config","deliberately","traces","board","layer","archive","locally","account","hosted","teams","paid","report","server","asks","host","slot","order","waits"]
hash: "2ab58729e4ceb17cf7ae47c865b0f503129728bb69df24682485aa19e59c69e0"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch run","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"},{"kind":"code","literal":"cap","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"},{"kind":"code","literal":"hev up","chunkId":"tradeoffs#work-is-decentralized-memory-is-shared"}]
sources: [{"chunkId":"tradeoffs#work-is-decentralized-memory-is-shared","url":"/docs/tradeoffs#work-is-decentralized-memory-is-shared","anchor":"work-is-decentralized-memory-is-shared"}]
---

Dispatch has no central scheduler: senders ask hosts for slots, full hosts queue orders first in first out, and there are no priorities, per-person limits or moving waiting orders; capacity is set in each sender's config. Memory is deliberately shared: traces and the board go to one Layer archive, run locally with no account or as hosted hev layer for teams, which is how kit is paid for.
