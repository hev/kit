---
id: "swarms#how-it-runs"
title: "Swarms"
heading: "How it runs"
group: "Use"
order: 352
url: "/docs/swarms#how-it-runs"
anchor: "how-it-runs"
terms: ["runs","starting","swarm","applies","orchestrator","writes","file","unit","table","state","directory","starts","supervisor","whose","tick","immediately","reconcile","interval","observes","repairs","failed","timed","dead","stalled","units","place","backoff","holds","notifies","repair","allowed","settles","races","launches","pending","replica","limit","idempotent","rounds","write"]
hash: "aa9b979e5fa6cc4e25268058bdb156627900e9cb0d39bfade53360881c317ed1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev swarm up -f FILE","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"swarm.md","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"swarm.json","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"~/dispatch/orch-NAME/","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"reconcile_every","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"done","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"failed","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"timeout","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"died","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"reply","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"effects: pr","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"blocked","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"needs-review","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"replicas","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"after","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"attempting","chunkId":"swarms#how-it-runs"},{"kind":"code","literal":"goal-met.json","chunkId":"swarms#how-it-runs"}]
sources: [{"chunkId":"swarms#how-it-runs","url":"/docs/swarms#how-it-runs","anchor":"how-it-runs"}]
---

Starting a swarm applies the orchestrator, writes the swarm file and unit table into its state directory, and starts a supervisor whose tick runs immediately and then at the reconcile interval. Each tick observes runs, repairs failed, timed-out, dead or stalled units in place with backoff, holds and notifies when repair is not allowed, settles races and launches pending units up to the replica limit; launches are idempotent, and rounds write no code and merge nothing.
