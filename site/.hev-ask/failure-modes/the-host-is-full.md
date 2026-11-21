---
id: "failure-modes#the-host-is-full"
title: "Failure modes"
heading: "The host is full"
group: "Operate"
order: 223
url: "/docs/failure-modes#the-host-is-full"
anchor: "the-host-is-full"
terms: ["host","full","error","order","queues","prints","position","starts","oldest","first","slot","frees","crashed","queue","restarts","daemon","next","scan","sweep","dispatch","stop","name","waits","take","while","orders","were","waiting","again"]
hash: "cd34f4277610f9839bafda83ad601cefc5924ae2525960345b7dfeddc15bc3bd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"failure-modes#the-host-is-full"}]
sources: [{"chunkId":"failure-modes#the-host-is-full","url":"/docs/failure-modes#the-host-is-full","anchor":"the-host-is-full"}]
---

A full host is not an error: the order queues, prints its position, and starts oldest-first when a slot frees. A crashed host's queue restarts on the daemon's next scan or a sweep.
