---
id: "swarms#how-it-runs"
title: "Swarms"
heading: "How it runs"
group: "Use"
order: 259
url: "/docs/swarms#how-it-runs"
anchor: "how-it-runs"
terms: ["runs","swarm","applies","orchestrator","writes","file","unit","table","directory","starts","supervisor","whose","tick","once","every","reconcile","interval","completes","ends","done","head","pushed","without","failed","timed","died","stalled","units","repaired","place","backoff","keeping","worktree","branch","slot","admission","checks","dispatch","json","orch"]
hash: "aa9b979e5fa6cc4e25268058bdb156627900e9cb0d39bfade53360881c317ed1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev swarm up -f FILE","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"swarm.md","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"swarm.json","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"~/dispatch/orch-NAME/","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"reconcile_every","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"done","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"failed","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"timeout","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"died","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"reply","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"effects: pr","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"blocked","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"needs-review","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"replicas","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"after","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"attempting","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"goal-met.json","chunkId":"swarms#how-it-runs"}]
sources: [{"chunkId":"swarms#how-it-runs","url":"/docs/swarms#how-it-runs","anchor":"how-it-runs"}]
---

swarm up applies the orchestrator, writes the swarm file and unit table into the orchestrator's run directory, and starts the supervisor, whose tick runs at once and then every reconcile interval. A unit completes when its run ends done with its head pushed; done without a pushed head is failed. Failed, timed-out, died or stalled units are repaired in place with a backoff, keeping worktree, branch and slot, and admission checks every dispatch.
