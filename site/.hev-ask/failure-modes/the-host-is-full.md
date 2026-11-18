---
id: "failure-modes#the-host-is-full"
title: "Failure modes"
heading: "The host is full"
group: "Operate"
order: 167
url: "/docs/failure-modes#the-host-is-full"
anchor: "the-host-is-full"
terms: ["host","full","error","order","waits","queue","dispatch","prints","position","starts","oldest","first","slot","frees","waiting","orders","show","listing","taken","stopping","after","crash","daemon","next","scan","manual","sweep","restarts","stop","name","take","crashed","while","were","again"]
hash: "cd34f4277610f9839bafda83ad601cefc5924ae2525960345b7dfeddc15bc3bd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"failure-modes#the-host-is-full"}]
sources: [{"chunkId":"failure-modes#the-host-is-full","url":"/docs/failure-modes#the-host-is-full","anchor":"the-host-is-full"}]
---

A full host is not an error: the order waits in that host's queue, dispatch prints its position, and it starts oldest-first when a slot frees. Waiting orders show in the run listing and can be taken out by stopping them; after a host crash the daemon's next scan or a manual sweep restarts the queue.
