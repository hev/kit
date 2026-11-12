---
id: "dispatch#refusals"
title: "Dispatch"
heading: "Refusals"
group: "Use"
order: 90
url: "/docs/dispatch#refusals"
anchor: "refusals"
terms: ["refusals","refused","dispatch","exits","error","line","name","already","exists","repo","admitted","unknown","missing","host","full","queues","instead","refusing","setup","failures","after","reservation","leave","cleanable","failed","record","dropped","connection","means","checking","before","retrying","cannot","create","clean","acme","buildbox","admission","repos","origin"]
hash: "28e095d3440c87fb614895ce357b14cad7f5c1e06bc5915e3ddcf1ab496b3d1a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error:","chunkId":"dispatch#refusals"},{"kind":"code","literal":"name already exists or cannot create","chunkId":"dispatch#refusals"},{"kind":"code","literal":"clean","chunkId":"dispatch#refusals"},{"kind":"code","literal":"repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"dispatch#refusals"},{"kind":"code","literal":"admission.repos","chunkId":"dispatch#refusals"},{"kind":"code","literal":"origin","chunkId":"dispatch#refusals"},{"kind":"code","literal":"unknown or missing host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"--host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"failed","chunkId":"dispatch#refusals"},{"kind":"code","literal":"ls --all","chunkId":"dispatch#refusals"},{"kind":"code","literal":"show","chunkId":"dispatch#refusals"},{"kind":"code","literal":"clean NAME","chunkId":"dispatch#refusals"},{"kind":"code","literal":"hev dispatch show NAME","chunkId":"dispatch#refusals"}]
sources: [{"chunkId":"dispatch#refusals","url":"/docs/dispatch#refusals","anchor":"refusals"}]
---

A refused dispatch exits 1 with one error line: name already exists, repo not admitted, or unknown or missing host. A full host queues instead of refusing, setup failures after reservation leave a cleanable failed record, and a dropped SSH connection means checking the run before retrying.
