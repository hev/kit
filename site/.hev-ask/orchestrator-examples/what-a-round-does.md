---
id: "orchestrator-examples#what-a-round-does"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 242
url: "/docs/orchestrator-examples#what-a-round-does"
anchor: "what-a-round-does"
terms: ["round","does","intake","supervisor","polls","source","code","everything","else","model","call","matching","event","without","receipt","renders","template","checks","scope","budget","dispatches","records","writes","checkpoint","stores","cursor","linear","events","agent","ready","every","minutes","runs","rest","called","whose","status","order","against","short"]
hash: "8f093cbee86dc7827f78cab4bff931df791eff0ac76f4b2ef50036fa9b7faf03"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-linear events","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"agent: \"off\"","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"agent-ready","chunkId":"orchestrator-examples#what-a-round-does"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#what-a-round-does"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does","url":"/docs/orchestrator-examples#what-a-round-does","anchor":"what-a-round-does"}]
---

In an intake round the supervisor polls the source and code does everything else with no model call: for each matching event without a receipt it renders the template, checks scope and budget, dispatches and records a receipt, then writes a checkpoint and stores the cursor.
