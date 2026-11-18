---
id: "dispatch#refusals"
title: "Dispatch"
heading: "Refusals"
group: "Use"
order: 152
url: "/docs/dispatch#refusals"
anchor: "refusals"
terms: ["refusals","refused","dispatch","exits","zero","error","line","name","already","exists","host","repo","admitted","unknown","missing","full","does","refuse","order","queues","setup","failures","after","slot","reserved","leave","failed","record","reason","cleaned","reuse","drops","outcome","inspect","before","retrying","cannot","create","clean","acme"]
hash: "28e095d3440c87fb614895ce357b14cad7f5c1e06bc5915e3ddcf1ab496b3d1a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error:","chunkId":"dispatch#refusals"},{"kind":"code","literal":"name already exists or cannot create","chunkId":"dispatch#refusals"},{"kind":"code","literal":"clean","chunkId":"dispatch#refusals"},{"kind":"code","literal":"repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"dispatch#refusals"},{"kind":"code","literal":"admission.repos","chunkId":"dispatch#refusals"},{"kind":"code","literal":"origin","chunkId":"dispatch#refusals"},{"kind":"code","literal":"unknown or missing host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"--host","chunkId":"dispatch#refusals"},{"kind":"code","literal":"failed","chunkId":"dispatch#refusals"},{"kind":"code","literal":"ls --all","chunkId":"dispatch#refusals"},{"kind":"code","literal":"show","chunkId":"dispatch#refusals"},{"kind":"code","literal":"clean NAME","chunkId":"dispatch#refusals"},{"kind":"code","literal":"hev dispatch show NAME","chunkId":"dispatch#refusals"}]
sources: [{"chunkId":"dispatch#refusals","url":"/docs/dispatch#refusals","anchor":"refusals"}]
---

A refused dispatch exits non-zero with one error line: the name already exists on the host, the repo is not admitted, or the host is unknown or missing. A full host does not refuse; the order queues. Setup failures after the slot is reserved leave a failed record with the reason that can be cleaned to reuse the name, and if SSH drops mid-dispatch the outcome is unknown, so inspect the run before retrying.
