---
id: "failure-modes#the-host-is-full"
title: "Failure modes"
heading: "The host is full"
group: "Operate"
order: 111
url: "/docs/failure-modes#the-host-is-full"
anchor: "the-host-is-full"
terms: ["host","full","queues","order","prints","position","instead","failing","starts","oldest","first","slot","frees","crashed","waiting","orders","restart","next","daemon","scan","sweep","dispatch","stop","name","error","waits","queue","take","while","were","again"]
hash: "cd34f4277610f9839bafda83ad601cefc5924ae2525960345b7dfeddc15bc3bd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"failure-modes#the-host-is-full"}]
sources: [{"chunkId":"failure-modes#the-host-is-full","url":"/docs/failure-modes#the-host-is-full","anchor":"the-host-is-full"}]
---

A full host queues the order and prints its position instead of failing; it starts oldest first when a slot frees. A crashed host's waiting orders restart on the next daemon scan or a sweep.
