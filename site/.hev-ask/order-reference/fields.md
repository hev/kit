---
id: "order-reference#fields"
title: "Order reference"
heading: "Fields"
group: "Reference"
order: 263
url: "/docs/order-reference#fields"
anchor: "fields"
terms: ["fields","reference","table","every","order","frontmatter","field","type","default","rules","name","host","repo","base","branch","harness","model","validation","preview","reply","issue","alias","repeat","interval","stop","condition","lifetime","timeout","minutes","context","recipe","options","trace","continuation","durations","syntax","without","units","repeating","rounds"]
hash: "ce903879a93ffd3978a4950c3a52dc4a57527806d3d1df1218923d10d8aed1d4"
mode: "source-primary"
facts: [{"kind":"code","literal":"name","chunkId":"order-reference#fields"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"order-reference#fields"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"order-reference#fields"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"order-reference#fields"},{"kind":"code","literal":"host","chunkId":"order-reference#fields"},{"kind":"code","literal":"hev dispatch ORDER.md","chunkId":"order-reference#fields"},{"kind":"code","literal":"unknown or missing host","chunkId":"order-reference#fields"},{"kind":"code","literal":"--host","chunkId":"order-reference#fields"},{"kind":"code","literal":"repo","chunkId":"order-reference#fields"},{"kind":"code","literal":"~/","chunkId":"order-reference#fields"},{"kind":"code","literal":"origin","chunkId":"order-reference#fields"},{"kind":"code","literal":"base","chunkId":"order-reference#fields"},{"kind":"code","literal":"main","chunkId":"order-reference#fields"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"order-reference#fields"},{"kind":"code","literal":"^[A-Za-z0-9][A-Za-z0-9/_.-]*$","chunkId":"order-reference#fields"},{"kind":"code","literal":"..","chunkId":"order-reference#fields"},{"kind":"code","literal":"harness","chunkId":"order-reference#fields"},{"kind":"code","literal":"codex","chunkId":"order-reference#fields"},{"kind":"code","literal":"claude","chunkId":"order-reference#fields"},{"kind":"code","literal":"model","chunkId":"order-reference#fields"},{"kind":"code","literal":"--model","chunkId":"order-reference#fields"},{"kind":"code","literal":"validation","chunkId":"order-reference#fields"},{"kind":"code","literal":"none","chunkId":"order-reference#fields"},{"kind":"code","literal":"browser","chunkId":"order-reference#fields"}]
sources: [{"chunkId":"order-reference#fields","url":"/docs/order-reference#fields","anchor":"fields"}]
---

Reference table of every order frontmatter field with type, default and rules: run name, host, repo, base branch, harness, model, validation and preview, reply, issue key and its alias, repeat interval with stop condition and lifetime, timeout in minutes, context recipe options and trace continuation. Durations use Go syntax without day units, repeating rounds reuse the branch and worktree, and a target that does not support context fields refuses the order.
