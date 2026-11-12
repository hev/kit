---
id: "orchestrator-examples#authority-and-budget"
title: "Orchestrator examples"
heading: "Authority and budget"
group: "Use"
order: 162
url: "/docs/orchestrator-examples#authority-and-budget"
anchor: "authority-and-budget"
terms: ["authority","budget","intake","loop","only","needs","dispatch","concurrent","daily","caps","stop","burst","labelled","issues","flooding","hosts","excess","events","wait","later","rounds","counts","reset","midnight","runs","starts","rework","retry","anything","concurrentruns","runsperday","keep","filling","reached","remaining","next","round","instead","failing"]
hash: "541e5e0e18dd9e8797743609799643dfc74b915c418ca2d5e4485f6759302e09"
mode: "agent-primary"
facts: [{"kind":"code","literal":"authority: [dispatch]","chunkId":"orchestrator-examples#authority-and-budget"},{"kind":"code","literal":"concurrent_runs: 2","chunkId":"orchestrator-examples#authority-and-budget"},{"kind":"code","literal":"runs_per_day: 5","chunkId":"orchestrator-examples#authority-and-budget"}]
sources: [{"chunkId":"orchestrator-examples#authority-and-budget","url":"/docs/orchestrator-examples#authority-and-budget","anchor":"authority-and-budget"}]
---

The intake loop only needs dispatch authority, and its concurrent and daily run caps stop a burst of labelled issues from flooding hosts; excess events wait for later rounds, and daily counts reset at midnight UTC.
