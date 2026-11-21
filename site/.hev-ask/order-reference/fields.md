---
id: "order-reference#fields"
title: "Order reference"
heading: "Fields"
group: "Reference"
order: 326
url: "/docs/order-reference#fields"
anchor: "fields"
terms: ["fields","field","table","name","required","lowercase","pattern","becomes","branch","directory","host","sender","config","wins","repo","clone","origin","base","default","main","other","defaults","rules","such","harness","timeout","reply","validation","preview","orders","dispatch","order","unknown","missing","codex","claude","model","effort","type","string"]
hash: "6eda76165b54b6d7fd76598e4031ec59cdaba7efdcb9cc71dc91ae4393781670"
mode: "source-primary"
facts: [{"kind":"code","literal":"name","chunkId":"order-reference#fields"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"order-reference#fields"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"order-reference#fields"},{"kind":"code","literal":"~/.hev/orders/\u003cname\u003e/","chunkId":"order-reference#fields"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"order-reference#fields"},{"kind":"code","literal":"host","chunkId":"order-reference#fields"},{"kind":"code","literal":"hev dispatch ORDER.md","chunkId":"order-reference#fields"},{"kind":"code","literal":"unknown or missing host","chunkId":"order-reference#fields"},{"kind":"code","literal":"--host","chunkId":"order-reference#fields"},{"kind":"code","literal":"repo","chunkId":"order-reference#fields"},{"kind":"code","literal":"~/","chunkId":"order-reference#fields"},{"kind":"code","literal":"origin","chunkId":"order-reference#fields"},{"kind":"code","literal":"base","chunkId":"order-reference#fields"},{"kind":"code","literal":"main","chunkId":"order-reference#fields"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"order-reference#fields"},{"kind":"code","literal":"^[A-Za-z0-9][A-Za-z0-9/_.-]*$","chunkId":"order-reference#fields"},{"kind":"code","literal":"..","chunkId":"order-reference#fields"},{"kind":"code","literal":"harness","chunkId":"order-reference#fields"},{"kind":"code","literal":"codex","chunkId":"order-reference#fields"},{"kind":"code","literal":"claude","chunkId":"order-reference#fields"},{"kind":"code","literal":"agy","chunkId":"order-reference#fields"},{"kind":"code","literal":"model","chunkId":"order-reference#fields"},{"kind":"code","literal":"--model","chunkId":"order-reference#fields"},{"kind":"code","literal":"effort","chunkId":"order-reference#fields"}]
sources: [{"chunkId":"order-reference#fields","url":"/docs/order-reference#fields","anchor":"fields"}]
---

Field table: name (required, lowercase pattern; becomes branch d/NAME and run directory), host (from sender's config; --host wins), repo (required clone with origin), base (default main), and other fields with defaults and rules such as harness, timeout, reply, validation, preview.
