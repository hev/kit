---
id: "orchestrator-examples#release-coordinator"
title: "Orchestrator examples"
heading: "Release coordinator"
group: "Use"
order: 221
url: "/docs/orchestrator-examples#release-coordinator"
anchor: "release-coordinator"
terms: ["release","coordinator","goal","based","orchestrator","spec","coordinates","train","finding","runs","tagged","issue","prefix","brief","never","merge","publish","rework","once","escalate","second","failure","retry","dead","report","ready","head","evidence","dispatch","notes","merges","disagree","keep","blockers","section","checkpoint","signal","context","stops","every"]
hash: "4b9543f2650420f76ada9bb528eddd180708f80ec2fc8740fe3122f2aab1f055"
mode: "agent-primary"
facts: [{"kind":"code","literal":"issue: train-0.7-...","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"issue: train-0.7-context","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"release-0-7.md","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"value","literal":"0.7","chunkId":"orchestrator-examples#release-coordinator"}]
sources: [{"chunkId":"orchestrator-examples#release-coordinator","url":"/docs/orchestrator-examples#release-coordinator","anchor":"release-coordinator"}]
---

A goal-based orchestrator spec that coordinates a release train by finding runs tagged with the train's issue prefix. Its brief: never merge, tag or publish; rework a red run once and escalate a second failure; retry dead runs once; report merge-ready PRs with head SHA and evidence; dispatch a release-notes fix when notes and merges disagree; keep a blockers section at the top of the checkpoint; signal when the goal is met.
