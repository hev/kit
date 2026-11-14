---
id: "hosts#capacity-and-the-queue"
title: "Hosts"
heading: "Capacity and the queue"
group: "Operate"
order: 169
url: "/docs/hosts#capacity-and-the-queue"
anchor: "capacity-and-the-queue"
terms: ["capacity","queue","many","runs","host","holds","once","counted","reserved","under","lock","senders","overfill","full","orders","wait","first","resumes","after","crashes","receiving","owns","default","config","only","lower","older","binaries","reject","level","setting","name","dispatch","sweep","stop","hosts","number","must","least","takes"]
hash: "a020c2779196aded29fb62fdce0a83520f615853280e2e63cd8930a1f51d4003"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"d-NAME","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch ls","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"cap = 8","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hosts.NAME.cap","chunkId":"hosts#capacity-and-the-queue"}]
sources: [{"chunkId":"hosts#capacity-and-the-queue","url":"/docs/hosts#capacity-and-the-queue","anchor":"capacity-and-the-queue"}]
---

Capacity is how many runs a host holds at once, counted on the host and reserved under a lock so senders can't overfill it. When full, orders wait in a first-in-first-out queue on the host that resumes after crashes. The receiving host owns its capacity in its default config; senders can only lower it, and older binaries reject the top-level setting.
