---
id: "swarms#orchestrator-fields"
title: "Swarms"
heading: "Orchestrator fields"
group: "Use"
order: 292
url: "/docs/swarms#orchestrator-fields"
anchor: "orchestrator-fields"
terms: ["orchestrator","fields","swarm","reuses","goal","reply","host","every","maxlifetime","harness","model","roundtimeout","notifyroute","budget","positive","concurrent","required","must","machine","added","reconcile","interval","restart","limit","backoff","ceiling","defaults","lifetime","round","timeout","notify","route","runs","these","names","default","concurrentruns","config","reaches","refused"]
hash: "3ac530343954fb86cf80313c3b03646a2c5f06112f602d806a2c30e83aa67901"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"reply","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"host","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"every","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"30m","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"max_lifetime","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"harness","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"model","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"round_timeout","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"notify_route","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"budget","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"budget.concurrent_runs","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"up","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"reconcile_every","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"2m","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"restart.limit","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"restart.backoff","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"restart.max_backoff","chunkId":"swarms#orchestrator-fields"}]
sources: [{"chunkId":"swarms#orchestrator-fields","url":"/docs/swarms#orchestrator-fields","anchor":"orchestrator-fields"}]
---

A swarm reuses orchestrator fields (goal, reply, host, every, maxlifetime, harness, model, roundtimeout, notifyroute, budget). Reply, maxlifetime and a positive concurrent-run budget are required, and host must be the machine you run up on. Added fields set the reconcile interval and restart limit, backoff and backoff ceiling, with defaults.
