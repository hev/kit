---
id: "orchestrator-examples#release-coordinator"
title: "Orchestrator examples"
heading: "Release coordinator"
group: "Use"
order: 296
url: "/docs/orchestrator-examples#release-coordinator"
anchor: "release-coordinator"
terms: ["release","coordinator","goal","based","orchestrator","coordinates","train","orders","carry","issue","merge","publish","authority","keeps","moving","between","person","publishing","draft","says","merged","green","notes","accurate","context","stops","every","order","carries","example","finds","runs","merges","publishes","those","acts","save","kind","name","docs"]
hash: "4b9543f2650420f76ada9bb528eddd180708f80ec2fc8740fe3122f2aab1f055"
mode: "agent-primary"
facts: [{"kind":"code","literal":"issue: train-0.7-...","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"issue: train-0.7-context","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"release-0-7.md","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"value","literal":"0.7","chunkId":"orchestrator-examples#release-coordinator"}]
sources: [{"chunkId":"orchestrator-examples#release-coordinator","url":"/docs/orchestrator-examples#release-coordinator","anchor":"release-coordinator"}]
---

Release coordinator: a goal-based orchestrator that coordinates a 0.7 release train (orders carry train issue IDs), with no merge or publish authority. It keeps the train moving between a person's merge of each PR and publishing the draft release, and says when each is due. The goal is all PRs merged with green CI and notes accurate.
