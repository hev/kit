---
id: "orchestrator-examples#what-it-asks-a-person"
title: "Orchestrator examples"
heading: "What it asks a person"
group: "Use"
order: 245
url: "/docs/orchestrator-examples#what-it-asks-a-person"
anchor: "what-it-asks-a-person"
terms: ["asks","person","agent","intake","loop","writes","prose","sends","line","notices","starts","stops","hits","event","rejected","needs","reconciling","readable","summary","second","notify","only","orchestrator","same","scope","reports","runs","started","finished","need","cannot","change","reply","laptop","report","authority","tells","reaches","carries","template"]
hash: "0d6cb77ebf350212b05be7923428d5fe1f674a4a6165036de58f11b51f543143"
mode: "agent-primary"
facts: [{"kind":"code","literal":"agent: \"off\"","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"reply: laptop/me","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"intake-report.md","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"authority: [notify]","chunkId":"orchestrator-examples#what-it-asks-a-person"}]
sources: [{"chunkId":"orchestrator-examples#what-it-asks-a-person","url":"/docs/orchestrator-examples#what-it-asks-a-person","anchor":"what-it-asks-a-person"}]
---

With the agent off, the intake loop writes no prose; kit sends one-line notices when it starts, stops, hits a cap or an event is rejected or needs reconciling. For a readable summary, run a second notify-only orchestrator over the same scope that reports which runs started, finished or need you and cannot change any run.
