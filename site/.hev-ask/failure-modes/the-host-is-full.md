---
id: "failure-modes#the-host-is-full"
title: "Failure modes"
heading: "The host is full"
group: "Operate"
order: 105
url: "/docs/failure-modes#the-host-is-full"
anchor: "the-host-is-full"
terms: ["host","full","error","order","waits","first","queue","removed","stop","restarted","after","crash","daemon","scan","sweep","dispatch","name","prints","position","starts","slot","frees","oldest","take","crashed","while","orders","were","waiting","next","again"]
hash: "cd34f4277610f9839bafda83ad601cefc5924ae2525960345b7dfeddc15bc3bd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ls","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch stop NAME","chunkId":"failure-modes#the-host-is-full"},{"kind":"code","literal":"hev dispatch sweep","chunkId":"failure-modes#the-host-is-full"}]
sources: [{"chunkId":"failure-modes#the-host-is-full","url":"/docs/failure-modes#the-host-is-full","anchor":"the-host-is-full"}]
---

A full host is not an error: the order waits in the host's first-in first-out queue, can be removed with stop, and is restarted after a crash by the daemon's scan or a sweep.
