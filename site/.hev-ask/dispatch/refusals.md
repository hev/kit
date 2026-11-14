---
id: "dispatch#refusals"
title: "Dispatch"
heading: "Refusals"
group: "Use"
order: 145
url: "/docs/dispatch#refusals"
anchor: "refusals"
terms: ["refusals","refused","dispatch","exits","line","error","common","causes","name","already","taken","host","repo","admitted","missing","unknown","full","queues","rather","refuses","setup","failures","after","reserving","slot","leave","failed","record","cleaned","drops","check","before","retrying","exists","cannot","create","clean","acme","buildbox","admission"]
hash: "28e095d3440c87fb614895ce357b14cad7f5c1e06bc5915e3ddcf1ab496b3d1a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error:","chunkId":"dispatch#refusals"},{"kind":"code","literal":"name already exists or cannot create","chunkId":"dispatch#refusals"},{"kind":"code","literal":"clean","chunkId":"dispatch#refusals"},{"kind":"code","literal":"repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"dispatch#refusals"},{"kind":"code","literal":"admission.repos","chunkId":"dispatch#refusals"},{"kind":"code","literal":"origin","chunkId":"dispatch#refusals"},{"kind":"code","literal":"unknown or missing host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"--host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"failed","chunkId":"dispatch#refusals"},{"kind":"code","literal":"ls --all","chunkId":"dispatch#refusals"},{"kind":"code","literal":"show","chunkId":"dispatch#refusals"},{"kind":"code","literal":"clean NAME","chunkId":"dispatch#refusals"},{"kind":"code","literal":"hev dispatch show NAME","chunkId":"dispatch#refusals"}]
sources: [{"chunkId":"dispatch#refusals","url":"/docs/dispatch#refusals","anchor":"refusals"}]
---

A refused dispatch exits 1 with a one-line error; common causes are a run name already taken on the host, a repo not admitted, or a missing or unknown host. A full host queues rather than refuses. Setup failures after reserving a slot leave a failed record that can be cleaned; if SSH drops mid-dispatch, check the run before retrying.
