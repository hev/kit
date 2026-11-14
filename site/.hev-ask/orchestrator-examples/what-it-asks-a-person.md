---
id: "orchestrator-examples#what-it-asks-a-person"
title: "Orchestrator examples"
heading: "What it asks a person"
group: "Use"
order: 235
url: "/docs/orchestrator-examples#what-it-asks-a-person"
anchor: "what-it-asks-a-person"
terms: ["asks","person","model","intake","orchestrator","only","sends","line","notices","start","stop","reached","rejected","events","reconciliation","needs","carries","reply","address","digest","second","notify","same","scope","compares","runs","last","checkpoint","reports","changes","calling","evidence","changed","agent","laptop","report","authority","writes","prose","tells"]
hash: "0d6cb77ebf350212b05be7923428d5fe1f674a4a6165036de58f11b51f543143"
mode: "agent-primary"
facts: [{"kind":"code","literal":"agent: \"off\"","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"reply: laptop/me","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"intake-report.md","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"authority: [notify]","chunkId":"orchestrator-examples#what-it-asks-a-person"}]
sources: [{"chunkId":"orchestrator-examples#what-it-asks-a-person","url":"/docs/orchestrator-examples#what-it-asks-a-person","anchor":"what-it-asks-a-person"}]
---

With the model off, the intake orchestrator only sends one-line notices on start, stop, cap reached, rejected events or reconciliation needs; each run carries the reply address. For a digest, run a second notify-only orchestrator over the same scope that compares runs with its last checkpoint and reports changes, calling the model only when evidence changed.
