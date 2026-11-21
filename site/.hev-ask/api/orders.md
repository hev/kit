---
id: "api/orders"
title: "Orders API"
heading: null
group: "API"
order: 43
url: "/docs/api/orders"
anchor: null
terms: ["orders","submits","dispatch","lists","reads","runs","amends","resolves","decisions","reworks","retries","stops","tails","logs","steers","live","rolling","club","train","until","stay","local","folder","name","steer","post","start","show","patch","amend","resolve","rework","retry","stop","delete","clean","submit","list","read","cancel"]
hash: "63f5b4d06a6fa359903b072c7f80ec5d32cd0cb7b118a7b96cc1db0c999d71c1"
mode: "source-primary"
facts: [{"kind":"code","literal":"/v1/orders","chunkId":"api/orders"},{"kind":"code","literal":"~/.hev/orders/\u003cname\u003e/","chunkId":"api/orders"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"api/orders"},{"kind":"code","literal":"hev dispatch","chunkId":"api/orders"},{"kind":"code","literal":"hev steer","chunkId":"api/orders"},{"kind":"code","literal":"hev-dispatch","chunkId":"api/orders"},{"kind":"code","literal":"GET /v1/orders","chunkId":"api/orders"},{"kind":"code","literal":"hev dispatch ls","chunkId":"api/orders"},{"kind":"code","literal":"POST /v1/orders","chunkId":"api/orders"},{"kind":"code","literal":"start","chunkId":"api/orders"},{"kind":"code","literal":"GET /v1/orders/{id}","chunkId":"api/orders"},{"kind":"code","literal":"hev dispatch show","chunkId":"api/orders"},{"kind":"code","literal":"PATCH /v1/orders/{id}","chunkId":"api/orders"},{"kind":"code","literal":"hev dispatch amend","chunkId":"api/orders"},{"kind":"code","literal":"say --resolve","chunkId":"api/orders"},{"kind":"code","literal":"rework","chunkId":"api/orders"},{"kind":"code","literal":"retry","chunkId":"api/orders"},{"kind":"code","literal":"stop","chunkId":"api/orders"},{"kind":"code","literal":"DELETE /v1/orders/{id}","chunkId":"api/orders"},{"kind":"code","literal":"hev dispatch stop","chunkId":"api/orders"},{"kind":"code","literal":"clean","chunkId":"api/orders"},{"kind":"code","literal":"GET /v1/orders/{id}/logs","chunkId":"api/orders"},{"kind":"code","literal":"hev dispatch logs","chunkId":"api/orders"},{"kind":"code","literal":"POST /v1/orders/{id}/steer","chunkId":"api/orders"}]
sources: [{"chunkId":"api/orders","url":"/docs/api/orders","anchor":null}]
---

The orders API submits dispatch orders, lists and reads runs, amends, resolves decisions, reworks, retries, stops, tails logs and steers live runs. Rolling out to Club Kit and Pro Kit on the 0.7 train; until then orders stay in a local folder.
