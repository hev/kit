---
id: "failure-modes#the-host-is-full"
title: "Failure modes"
heading: "The host is full"
group: "Operate"
order: 160
url: "/docs/failure-modes#the-host-is-full"
anchor: "the-host-is-full"
terms: ["host","full","error","order","waits","oldest","first","queue","starts","slot","frees","removed","stopping","restarted","after","crash","daemon","scan","sweep","dispatch","stop","name","prints","position","take","crashed","while","orders","were","waiting","next","again"]
hash: "cd34f4277610f9839bafda83ad601cefc5924ae2525960345b7dfeddc15bc3bd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"failure-modes#the-host-is-full"}]
sources: [{"chunkId":"failure-modes#the-host-is-full","url":"/docs/failure-modes#the-host-is-full","anchor":"the-host-is-full"}]
---

A full host is not an error: the order waits in that host's oldest-first queue and starts when a slot frees, can be removed by stopping it, and is restarted after a crash by the daemon's scan or a sweep.
