---
id: "orchestrator-examples#what-it-asks-a-person"
title: "Orchestrator examples"
heading: "What it asks a person"
group: "Use"
order: 308
url: "/docs/orchestrator-examples#what-it-asks-a-person"
anchor: "what-it-asks-a-person"
terms: ["asks","person","agent","intake","orchestrator","writes","prose","tells","reply","address","line","start","stop","reached","rejected","events","started","carries","template","summary","second","report","only","same","scope","example","spec","laptop","authority","notify","starts","stops","reaches","event","needs","reconciling","command","hears","ends","message"]
hash: "0d6cb77ebf350212b05be7923428d5fe1f674a4a6165036de58f11b51f543143"
mode: "agent-primary"
facts: [{"kind":"code","literal":"agent: \"off\"","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"reply: laptop/me","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"intake-report.md","chunkId":"orchestrator-examples#what-it-asks-a-person"},{"kind":"code","literal":"authority: [notify]","chunkId":"orchestrator-examples#what-it-asks-a-person"}]
sources: [{"chunkId":"orchestrator-examples#what-it-asks-a-person","url":"/docs/orchestrator-examples#what-it-asks-a-person","anchor":"what-it-asks-a-person"}]
---

With agent off the intake orchestrator writes no prose; kit tells the reply address in one line on start, stop, cap reached or rejected events, and each started run carries the template's reply. For a summary, run a second report-only orchestrator over the same scope (example spec intake-report).
