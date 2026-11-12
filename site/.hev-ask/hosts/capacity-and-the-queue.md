---
id: "hosts#capacity-and-the-queue"
title: "Hosts"
heading: "Capacity and the queue"
group: "Operate"
order: 114
url: "/docs/hosts#capacity-and-the-queue"
anchor: "capacity-and-the-queue"
terms: ["capacity","queue","many","runs","host","holds","once","counted","itself","reserved","under","lock","senders","cannot","overfill","full","hosts","orders","first","restarted","after","crashes","daemon","sweep","target","owns","config","only","lower","repeating","hold","slots","between","rounds","name","dispatch","stop","number","must","least"]
hash: "a020c2779196aded29fb62fdce0a83520f615853280e2e63cd8930a1f51d4003"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"d-NAME","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch ls","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"cap = 8","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hosts.NAME.cap","chunkId":"hosts#capacity-and-the-queue"}]
sources: [{"chunkId":"hosts#capacity-and-the-queue","url":"/docs/hosts#capacity-and-the-queue","anchor":"capacity-and-the-queue"}]
---

Cap is how many runs a host holds at once, counted on the host itself and reserved under a lock so senders cannot overfill it. Full hosts queue orders first in, first out, restarted after crashes by the daemon or a sweep; the target owns capacity in its own config and senders can only lower it, and repeating orders hold slots between rounds.
