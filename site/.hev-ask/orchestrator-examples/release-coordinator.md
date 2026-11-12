---
id: "orchestrator-examples#release-coordinator"
title: "Orchestrator examples"
heading: "Release coordinator"
group: "Use"
order: 171
url: "/docs/orchestrator-examples#release-coordinator"
anchor: "release-coordinator"
terms: ["release","coordinator","goal","based","orchestrator","keeps","train","moving","finding","runs","issue","prefix","merge","publish","authority","brief","reworks","once","retries","dead","ones","reports","ready","evidence","dispatches","notes","needed","blockers","atop","checkpoint","declares","merged","match","context","stops","coordinates","every","order","carries","example"]
hash: "4b9543f2650420f76ada9bb528eddd180708f80ec2fc8740fe3122f2aab1f055"
mode: "agent-primary"
facts: [{"kind":"code","literal":"issue: train-0.7-...","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"issue: train-0.7-context","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"release-0-7.md","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"value","literal":"0.7","chunkId":"orchestrator-examples#release-coordinator"}]
sources: [{"chunkId":"orchestrator-examples#release-coordinator","url":"/docs/orchestrator-examples#release-coordinator","anchor":"release-coordinator"}]
---

A goal-based orchestrator that keeps a release train moving, finding runs by an issue prefix, with no merge or publish authority. Its brief reworks red runs once, retries dead ones, reports merge-ready PRs with evidence, dispatches a release notes fix when needed, keeps blockers atop the checkpoint, and declares the goal met when all PRs merged and notes match.
