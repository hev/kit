---
id: "swarms#orchestrator-fields"
title: "Swarms"
heading: "Orchestrator fields"
group: "Use"
order: 350
url: "/docs/swarms#orchestrator-fields"
anchor: "orchestrator-fields"
terms: ["orchestrator","fields","swarms","reuse","such","goal","reply","interval","lifetime","harness","model","budget","positive","concurrency","required","host","named","must","swarm","started","specific","reconcile","restart","limit","backoff","ceiling","unit","every","round","timeout","notify","route","concurrent","runs","these","names","default","maxlifetime","roundtimeout","notifyroute"]
hash: "3ac530343954fb86cf80313c3b03646a2c5f06112f602d806a2c30e83aa67901"
mode: "agent-primary"
facts: [{"kind":"code","literal":"goal","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"reply","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"host","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"every","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"30m","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"max_lifetime","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"harness","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"model","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"round_timeout","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"notify_route","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"budget","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"budget.concurrent_runs","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"up","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"reconcile_every","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"2m","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"restart.limit","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"restart.backoff","chunkId":"swarms#orchestrator-fields"},{"kind":"code","literal":"restart.max_backoff","chunkId":"swarms#orchestrator-fields"}]
sources: [{"chunkId":"swarms#orchestrator-fields","url":"/docs/swarms#orchestrator-fields","anchor":"orchestrator-fields"}]
---

Swarms reuse orchestrator fields such as goal, reply, interval, lifetime, harness, model and budget; reply, lifetime and a positive concurrency budget are required, and any host named must be the one the swarm is started on. Swarm-specific fields set the reconcile interval and the restart limit, backoff and backoff ceiling per unit.
