---
id: "orchestrator-examples#how-duplicates-are-avoided"
title: "Orchestrator examples"
heading: "How duplicates are avoided"
group: "Use"
order: 293
url: "/docs/orchestrator-examples#how-duplicates-are-avoided"
anchor: "how-duplicates-are-avoided"
terms: ["duplicates","avoided","duplicate","avoidance","intake","loop","receipts","keyed","source","event","action","mode","prevent","dispatching","however","often","polled","defines","newness","issue","gives","ever","include","transition","allow","entry","names","deterministic","hash","orders","orch","markers","attempting","needs","reconcile","orchestrate","admitted","receipt","kept","already"]
hash: "df9e569f30133629a7cd95f7933527fee99a25c2d37dcf7a2057ecc9ffa72f51"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/orders/orch-intake/source-markers/","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"src-intake-...","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"attempting","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"needs-reconcile","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"hev orchestrate reconcile intake --event ABC-12 --admitted","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"--not-admitted","chunkId":"orchestrator-examples#how-duplicates-are-avoided"}]
sources: [{"chunkId":"orchestrator-examples#how-duplicates-are-avoided","url":"/docs/orchestrator-examples#how-duplicates-are-avoided","anchor":"how-duplicates-are-avoided"}]
---

Duplicate avoidance in the intake loop: receipts keyed by source, event ID, action and mode prevent re-dispatching an event however often polled; the event ID defines newness (issue key gives one run per issue ever, or include a transition ID to allow re-entry). Run names are deterministic from a hash.
