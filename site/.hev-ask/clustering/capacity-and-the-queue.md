---
id: "clustering#capacity-and-the-queue"
title: "Clustering"
heading: "Capacity and the queue"
group: "Operate"
order: 114
url: "/docs/clustering#capacity-and-the-queue"
anchor: "capacity-and-the-queue"
terms: ["capacity","queue","many","runs","host","holds","once","least","slot","admission","until","supervisor","exits","counted","tmux","sessions","unix","pids","windows","excess","orders","first","name","dispatch","sweep","stop","hosts","number","must","takes","live","slots","itself","named","checked","process","reserves","under","short","lock"]
hash: "88ff860d0f938c3b844672fac6dc3466c44330d00f21f0f2196ef353e1feb40e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap","chunkId":"clustering#capacity-and-the-queue"},{"kind":"code","literal":"d-NAME","chunkId":"clustering#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"clustering#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch ls","chunkId":"clustering#capacity-and-the-queue"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"clustering#capacity-and-the-queue"},{"kind":"code","literal":"cap = 8","chunkId":"clustering#capacity-and-the-queue"},{"kind":"code","literal":"hosts.NAME.cap","chunkId":"clustering#capacity-and-the-queue"}]
sources: [{"chunkId":"clustering#capacity-and-the-queue","url":"/docs/clustering#capacity-and-the-queue","anchor":"capacity-and-the-queue"}]
---

Cap is how many runs a host holds at once (at least 1); a run holds a slot from admission until its supervisor exits, counted from tmux sessions on Unix or supervisor PIDs on Windows. Excess orders queue first in, first out.
