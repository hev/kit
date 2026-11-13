---
id: "orchestrator-examples#authority-and-budget"
title: "Orchestrator examples"
heading: "Authority and budget"
group: "Use"
order: 168
url: "/docs/orchestrator-examples#authority-and-budget"
anchor: "authority-and-budget"
terms: ["authority","budget","intake","loop","needs","only","dispatch","start","runs","concurrency","daily","caps","stop","burst","labelled","issues","filling","hosts","extra","events","waiting","next","round","counts","reset","midnight","concurrent","starts","rework","retry","anything","concurrentruns","runsperday","keep","reached","remaining","wait","instead","failing"]
hash: "541e5e0e18dd9e8797743609799643dfc74b915c418ca2d5e4485f6759302e09"
mode: "agent-primary"
facts: [{"kind":"code","literal":"authority: [dispatch]","chunkId":"orchestrator-examples#authority-and-budget"},{"kind":"code","literal":"concurrent_runs: 2","chunkId":"orchestrator-examples#authority-and-budget"},{"kind":"code","literal":"runs_per_day: 5","chunkId":"orchestrator-examples#authority-and-budget"}]
sources: [{"chunkId":"orchestrator-examples#authority-and-budget","url":"/docs/orchestrator-examples#authority-and-budget","anchor":"authority-and-budget"}]
---

The intake loop needs only dispatch authority, so it can only start runs. Concurrency and daily caps stop a burst of labelled issues from filling hosts, with extra events waiting for the next round. Daily counts reset at midnight UTC.
