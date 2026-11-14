---
id: "orchestrator-examples#authority-and-budget"
title: "Orchestrator examples"
heading: "Authority and budget"
group: "Use"
order: 212
url: "/docs/orchestrator-examples#authority-and-budget"
anchor: "authority-and-budget"
terms: ["authority","budget","intake","orchestrator","only","needs","dispatch","because","starts","runs","small","concurrency","daily","caps","keep","burst","labelled","issues","flooding","hosts","events","wait","next","round","counts","reset","midnight","concurrent","rework","retry","anything","concurrentruns","runsperday","filling","reached","remaining","instead","failing"]
hash: "541e5e0e18dd9e8797743609799643dfc74b915c418ca2d5e4485f6759302e09"
mode: "agent-primary"
facts: [{"kind":"code","literal":"authority: [dispatch]","chunkId":"orchestrator-examples#authority-and-budget"},{"kind":"code","literal":"concurrent_runs: 2","chunkId":"orchestrator-examples#authority-and-budget"},{"kind":"code","literal":"runs_per_day: 5","chunkId":"orchestrator-examples#authority-and-budget"}]
sources: [{"chunkId":"orchestrator-examples#authority-and-budget","url":"/docs/orchestrator-examples#authority-and-budget","anchor":"authority-and-budget"}]
---

The intake orchestrator only needs dispatch authority because it only starts runs. Small concurrency and daily caps keep a burst of labelled issues from flooding hosts; events over the cap wait for the next round, and daily counts reset at midnight UTC.
