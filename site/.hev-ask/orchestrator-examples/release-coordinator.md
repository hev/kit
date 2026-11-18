---
id: "orchestrator-examples#release-coordinator"
title: "Orchestrator examples"
heading: "Release coordinator"
group: "Use"
order: 230
url: "/docs/orchestrator-examples#release-coordinator"
anchor: "release-coordinator"
terms: ["release","coordinator","goal","based","orchestrator","train","finds","runs","shared","issue","prefix","reworks","once","retries","dead","ones","reports","merge","ready","evidence","dispatches","notes","merges","disagree","keeps","blockers","list","checkpoint","never","tags","publishes","person","does","both","stops","context","coordinates","every","order","carries"]
hash: "4b9543f2650420f76ada9bb528eddd180708f80ec2fc8740fe3122f2aab1f055"
mode: "agent-primary"
facts: [{"kind":"code","literal":"issue: train-0.7-...","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"issue: train-0.7-context","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"release-0-7.md","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"value","literal":"0.7","chunkId":"orchestrator-examples#release-coordinator"}]
sources: [{"chunkId":"orchestrator-examples#release-coordinator","url":"/docs/orchestrator-examples#release-coordinator","anchor":"release-coordinator"}]
---

A goal-based orchestrator for a release train: it finds the train's runs by a shared issue prefix, reworks a red run once, retries dead ones, reports merge-ready PRs with evidence, dispatches a release-notes fix when notes and merges disagree, and keeps a blockers list at the top of its checkpoint. It never merges, tags or publishes; a person does both, and it stops when the goal is met.
