---
id: "orchestrator-examples#how-duplicates-are-avoided"
title: "Orchestrator examples"
heading: "How duplicates are avoided"
group: "Use"
order: 226
url: "/docs/orchestrator-examples#how-duplicates-are-avoided"
anchor: "how-duplicates-are-avoided"
terms: ["duplicates","avoided","intake","loop","avoids","duplicate","runs","event","receipt","deterministic","names","host","refuses","start","twice","never","resending","uncertain","launches","defines","counts","issue","here","means","ever","launch","unconfirmed","after","fifteen","minutes","marked","manual","reconciliation","dispatch","orch","source","markers","attempting","needs","reconcile"]
hash: "edd465fc2e32ad38a7a50103f63899a7d7e1cf32e5d378906700e57651945a8c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/dispatch/orch-intake/source-markers/","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"src-intake-...","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"attempting","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"needs-reconcile","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"hev orchestrate reconcile intake --event ABC-12 --admitted","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"--not-admitted","chunkId":"orchestrator-examples#how-duplicates-are-avoided"}]
sources: [{"chunkId":"orchestrator-examples#how-duplicates-are-avoided","url":"/docs/orchestrator-examples#how-duplicates-are-avoided","anchor":"how-duplicates-are-avoided"}]
---

The intake loop avoids duplicate runs with a per-event receipt, deterministic run names that the host refuses to start twice, and never resending uncertain launches. The event ID defines what counts as new (the issue key here means one run per issue ever), and a launch unconfirmed after fifteen minutes is marked for manual reconciliation.
