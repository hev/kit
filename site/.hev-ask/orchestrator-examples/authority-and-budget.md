---
id: "orchestrator-examples#authority-and-budget"
title: "Orchestrator examples"
heading: "Authority and budget"
group: "Use"
order: 289
url: "/docs/orchestrator-examples#authority-and-budget"
anchor: "authority-and-budget"
terms: ["authority","budget","intake","only","dispatch","start","runs","concurrent","caps","keep","bursts","filling","hosts","events","wait","next","round","daily","counts","reset","midnight","needs","starts","rework","retry","anything","concurrentruns","runsperday","burst","labelled","issues","reached","remaining","instead","failing"]
hash: "541e5e0e18dd9e8797743609799643dfc74b915c418ca2d5e4485f6759302e09"
mode: "agent-primary"
facts: [{"kind":"code","literal":"authority: [dispatch]","chunkId":"orchestrator-examples#authority-and-budget"},{"kind":"code","literal":"concurrent_runs: 2","chunkId":"orchestrator-examples#authority-and-budget"},{"kind":"code","literal":"runs_per_day: 5","chunkId":"orchestrator-examples#authority-and-budget"}]
sources: [{"chunkId":"orchestrator-examples#authority-and-budget","url":"/docs/orchestrator-examples#authority-and-budget","anchor":"authority-and-budget"}]
---

Intake authority and budget: only dispatch authority, so it can only start runs; concurrent run and per-day caps keep bursts from filling hosts, and events over the cap wait for the next round. Daily counts reset at midnight UTC.
