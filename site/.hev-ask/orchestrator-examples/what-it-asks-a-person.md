---
id: "orchestrator-examples#what-it-asks-a-person"
title: "Orchestrator examples"
heading: "What it asks a person"
group: "Use"
order: 185
url: "/docs/orchestrator-examples#what-it-asks-a-person"
anchor: "what-it-asks-a-person"
terms: ["asks","person","model","intake","loop","only","sends","line","notices","start","stop","caps","rejections","reconciliation","reply","reaches","notify","command","summary","second","orchestrator","same","scope","reports","changes","hour","cannot","alter","runs","agent","laptop","report","authority","writes","prose","tells","starts","stops","event","rejected"]
hash: "0d6cb77ebf350212b05be7923428d5fe1f674a4a6165036de58f11b51f543143"
mode: "agent-primary"
facts: [{"kind":"code","literal":"agent: \"off\"","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"reply: laptop/me","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"intake-report.md","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"authority: [notify]","chunkId":"orchestrator-examples#what-it-asks-a-person"}]
sources: [{"chunkId":"orchestrator-examples#what-it-asks-a-person","url":"/docs/orchestrator-examples#what-it-asks-a-person","anchor":"what-it-asks-a-person"}]
---

With no model the intake loop only sends one-line notices on start, stop, caps, rejections or reconciliation, and each run's reply reaches your notify command. For a summary, run a second notify-only orchestrator over the same scope that reports changes each hour and cannot alter runs.
