---
id: "orchestrator-examples#how-duplicates-are-avoided"
title: "Orchestrator examples"
heading: "How duplicates are avoided"
group: "Use"
order: 166
url: "/docs/orchestrator-examples#how-duplicates-are-avoided"
anchor: "how-duplicates-are-avoided"
terms: ["duplicates","avoided","intake","loop","avoids","receipt","event","keyed","source","action","mode","deterministic","names","hosts","refuse","start","twice","never","resending","uncertain","launches","unconfirmed","become","needs","reconcile","after","minutes","settled","hand","dispatch","orch","markers","attempting","orchestrate","admitted","receipts","kept","already","dispatched","again"]
hash: "edd465fc2e32ad38a7a50103f63899a7d7e1cf32e5d378906700e57651945a8c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/dispatch/orch-intake/source-markers/","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"src-intake-...","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"attempting","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"needs-reconcile","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"hev orchestrate reconcile intake --event ABC-12 --admitted","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"--not-admitted","chunkId":"orchestrator-examples#how-duplicates-are-avoided"}]
sources: [{"chunkId":"orchestrator-examples#how-duplicates-are-avoided","url":"/docs/orchestrator-examples#how-duplicates-are-avoided","anchor":"how-duplicates-are-avoided"}]
---

The intake loop avoids duplicates with a receipt per event keyed by source, event ID, action and mode, deterministic run names that hosts refuse to start twice, and never resending uncertain launches. Unconfirmed launches become needs-reconcile after 15 minutes and are settled by hand.
