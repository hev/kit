---
id: "orchestrator-examples#release-coordinator"
title: "Orchestrator examples"
heading: "Release coordinator"
group: "Use"
order: 176
url: "/docs/orchestrator-examples#release-coordinator"
anchor: "release-coordinator"
terms: ["release","coordinator","goal","based","orchestrator","coordinating","train","finding","runs","issue","prefix","merge","publish","authority","keeps","moving","between","human","acts","merging","publishing","spec","host","every","minutes","days","brief","telling","full","file","given","context","stops","coordinates","order","carries","example","finds","person","merges"]
hash: "4b9543f2650420f76ada9bb528eddd180708f80ec2fc8740fe3122f2aab1f055"
mode: "agent-primary"
facts: [{"kind":"code","literal":"issue: train-0.7-...","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"issue: train-0.7-context","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"code","literal":"release-0-7.md","chunkId":"orchestrator-examples#release-coordinator"},{"kind":"value","literal":"0.7","chunkId":"orchestrator-examples#release-coordinator"}]
sources: [{"chunkId":"orchestrator-examples#release-coordinator","url":"/docs/orchestrator-examples#release-coordinator","anchor":"release-coordinator"}]
---

A goal-based orchestrator coordinating a 0.7 release train, finding runs by an issue prefix. It has no merge or publish authority and keeps the train moving between two human acts, merging and publishing. The spec runs on a host every 30 minutes up to 14 days with a brief telling it what to do. A full spec file is given.
