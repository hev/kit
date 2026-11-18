---
id: "hosts#capacity-and-the-queue"
title: "Hosts"
heading: "Capacity and the queue"
group: "Operate"
order: 176
url: "/docs/hosts#capacity-and-the-queue"
anchor: "capacity-and-the-queue"
terms: ["capacity","queue","host","many","runs","holds","once","slot","admission","until","supervisor","exits","slots","reserved","under","lock","senders","cannot","overfill","every","eligible","full","order","waits","first","receiving","started","finishing","supervisors","recovered","daemon","scan","after","crash","target","owns","limit","default","sender","only"]
hash: "a020c2779196aded29fb62fdce0a83520f615853280e2e63cd8930a1f51d4003"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"d-NAME","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch ls","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"cap = 8","chunkId":"hosts#capacity-and-the-queue"},{"kind":"code","literal":"hosts.NAME.cap","chunkId":"hosts#capacity-and-the-queue"}]
sources: [{"chunkId":"hosts#capacity-and-the-queue","url":"/docs/hosts#capacity-and-the-queue","anchor":"capacity-and-the-queue"}]
---

A host's cap is how many runs it holds at once; a run holds a slot from admission until its supervisor exits, and slots are reserved under a lock so senders cannot overfill it. When every eligible host is full the order waits in a first-in, first-out queue on the receiving host, started by finishing supervisors or recovered by the daemon's scan after a crash. The target owns the limit (default 2) and a sender can only lower it; size it to what the machine can carry, upgrade binaries before adding the top-level setting, and note repeating orders hold their slot between rounds.
