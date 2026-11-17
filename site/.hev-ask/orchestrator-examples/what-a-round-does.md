---
id: "orchestrator-examples#what-a-round-does"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 188
url: "/docs/orchestrator-examples#what-a-round-does"
anchor: "what-a-round-does"
terms: ["round","does","every","minutes","supervisor","runs","source","events","call","code","model","renders","order","matching","event","checks","scope","budget","dispatches","records","receipt","writes","checkpoint","cursor","linear","agent","ready","rest","called","whose","status","against","short","stores"]
hash: "8f093cbee86dc7827f78cab4bff931df791eff0ac76f4b2ef50036fa9b7faf03"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-linear events","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"agent: \"off\"","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"agent-ready","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#what-a-round-does"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does","url":"/docs/orchestrator-examples#what-a-round-does","anchor":"what-a-round-does"}]
---

Every 15 minutes the supervisor runs the source's events call and code, with no model, renders the order for each new matching event, checks scope and budget, dispatches and records a receipt, then writes a checkpoint and cursor.
