---
id: "failure-modes#the-name-is-taken"
title: "Failure modes"
heading: "The name is taken"
group: "Operate"
order: 114
url: "/docs/failure-modes#the-name-is-taken"
anchor: "the-name-is-taken"
terms: ["name","taken","names","unique","host","including","finished","failed","runs","pick","clean","record","subcommand","checks","every","reachable","first","error","already","exists","cannot","create","dispatch","remove","fails"]
hash: "33c857897d0b523f461663221f40e439f61f86639caff63d88d4a3f86a7df91f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: name already exists or cannot create: ...","chunkId":"failure-modes#the-name-is-taken"},{"kind":"code","literal":"name","chunkId":"failure-modes#the-name-is-taken"},{"kind":"code","literal":"hev dispatch clean NAME","chunkId":"failure-modes#the-name-is-taken"},{"kind":"code","literal":"run","chunkId":"failure-modes#the-name-is-taken"},{"kind":"code","literal":"name already exists: NAME","chunkId":"failure-modes#the-name-is-taken"}]
sources: [{"chunkId":"failure-modes#the-name-is-taken","url":"/docs/failure-modes#the-name-is-taken","anchor":"the-name-is-taken"}]
---

Run names are unique per host, including finished and failed runs. Pick a new name or clean the old record. The run subcommand checks names on every reachable host first.
