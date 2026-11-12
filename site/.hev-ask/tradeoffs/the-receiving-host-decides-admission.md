---
id: "tradeoffs#the-receiving-host-decides-admission"
title: "Tradeoffs"
heading: "The receiving host decides admission"
group: "Reference"
order: 295
url: "/docs/tradeoffs#the-receiving-host-decides-admission"
anchor: "the-receiving-host-decides-admission"
terms: ["receiving","host","decides","admission","config","holds","list","repos","accepts","checks","clone","actual","origin","senders","cannot","widen","bypass","cost","machine","policy","central","server","single","view","across","hosts","lives","sender","rename","around","point","different","file","change","edit","files","there","every","place"]
hash: "11eb04a9485e2146651a2206b9c714f0ac5523dc1cfac963a755bfcf5e1b4b87"
mode: "source-primary"
facts: [{"kind":"code","literal":"origin","chunkId":"tradeoffs#the-receiving-host-decides-admission"}]
sources: [{"chunkId":"tradeoffs#the-receiving-host-decides-admission","url":"/docs/tradeoffs#the-receiving-host-decides-admission","anchor":"the-receiving-host-decides-admission"}]
---

Each host's own config holds the list of repos it accepts and checks the clone's actual origin, so senders cannot widen or bypass it. The cost is per-machine policy with no central server or single view across hosts.
