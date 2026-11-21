---
id: "orchestrator-examples#what-a-round-does"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 305
url: "/docs/orchestrator-examples#what-a-round-does"
anchor: "what-a-round-does"
terms: ["round","does","intake","loop","every","minutes","supervisor","runs","source","events","call","code","model","renders","template","agent","ready","event","checks","scope","budget","dispatches","records","receipt","writes","short","checkpoint","stores","cursor","linear","rest","called","whose","status","order","against"]
hash: "8f093cbee86dc7827f78cab4bff931df791eff0ac76f4b2ef50036fa9b7faf03"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-linear events","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"agent: \"off\"","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"agent-ready","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#what-a-round-does"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does","url":"/docs/orchestrator-examples#what-a-round-does","anchor":"what-a-round-does"}]
---

A round of the intake loop: every 15 minutes the supervisor runs the source events call, then code (no model) renders the template for each new agent-ready event, checks scope and budget, dispatches, records a receipt, writes a short checkpoint and stores the cursor.
