---
id: "order-reference#fields"
title: "Order reference"
heading: "Fields"
group: "Reference"
order: 203
url: "/docs/order-reference#fields"
anchor: "fields"
terms: ["fields","order","required","name","becomes","branch","directory","host","repo","base","harness","model","validation","preview","reply","issue","alias","repeating","cadence","stop","condition","lifetime","timeout","minutes","context","recipe","path","trace","continue","durations","take","units","without","days","hosts","know","refuse","dispatch","unknown","missing"]
hash: "4860368c1fefdd8942db27cc824beaec3e4ce2907adb5be9fa6b8826f69a35e6"
mode: "source-primary"
facts: [{"kind":"code","literal":"name","chunkId":"order-reference#fields"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"order-reference#fields"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"order-reference#fields"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"order-reference#fields"},{"kind":"code","literal":"host","chunkId":"order-reference#fields"},{"kind":"code","literal":"hev dispatch ORDER.md","chunkId":"order-reference#fields"},{"kind":"code","literal":"unknown or missing host","chunkId":"order-reference#fields"},{"kind":"code","literal":"--host","chunkId":"order-reference#fields"},{"kind":"code","literal":"repo","chunkId":"order-reference#fields"},{"kind":"code","literal":"~/","chunkId":"order-reference#fields"},{"kind":"code","literal":"origin","chunkId":"order-reference#fields"},{"kind":"code","literal":"base","chunkId":"order-reference#fields"},{"kind":"code","literal":"main","chunkId":"order-reference#fields"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"order-reference#fields"},{"kind":"code","literal":"^[A-Za-z0-9][A-Za-z0-9/_.-]*$","chunkId":"order-reference#fields"},{"kind":"code","literal":"..","chunkId":"order-reference#fields"},{"kind":"code","literal":"harness","chunkId":"order-reference#fields"},{"kind":"code","literal":"codex","chunkId":"order-reference#fields"},{"kind":"code","literal":"claude","chunkId":"order-reference#fields"},{"kind":"code","literal":"model","chunkId":"order-reference#fields"},{"kind":"code","literal":"--model","chunkId":"order-reference#fields"},{"kind":"code","literal":"validation","chunkId":"order-reference#fields"},{"kind":"code","literal":"none","chunkId":"order-reference#fields"},{"kind":"code","literal":"browser","chunkId":"order-reference#fields"}]
sources: [{"chunkId":"order-reference#fields","url":"/docs/order-reference#fields","anchor":"fields"}]
---

Order fields: a required name that becomes the branch and run directory, host, required repo, base branch, harness, model, validation, preview, reply, issue or its alias, repeating cadence with stop condition and lifetime, timeout in minutes, a context recipe path, and a trace to continue. Durations take Go units without days, and hosts that do not know the context key refuse it.
