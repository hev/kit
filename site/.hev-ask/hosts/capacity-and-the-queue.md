---
id: "hosts#capacity-and-the-queue"
title: "Hosts"
heading: "Capacity and the queue"
group: "Operate"
order: 120
url: "/docs/hosts#capacity-and-the-queue"
anchor: "capacity-and-the-queue"
terms: ["capacity","queue","many","runs","host","holds","once","counted","itself","admission","reserves","slot","under","short","lock","senders","cannot","overfill","full","queues","orders","first","name","dispatch","sweep","stop","hosts","number","must","least","takes","until","supervisor","exits","live","slots","tmux","sessions","named","unix"]
hash: "a020c2779196aded29fb62fdce0a83520f615853280e2e63cd8930a1f51d4003"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"d-NAME","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch ls","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"cap = 8","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hosts.NAME.cap","chunkId":"hosts#capacity-and-the-queue"}]
sources: [{"chunkId":"hosts#capacity-and-the-queue","url":"/docs/hosts#capacity-and-the-queue","anchor":"capacity-and-the-queue"}]
---

Capacity is how many runs a host holds at once, counted on the host itself. Admission reserves a slot under a short lock, so senders cannot overfill, and a full host queues orders first in first out.
