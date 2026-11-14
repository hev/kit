---
id: "orchestrator-examples#how-duplicates-are-avoided"
title: "Orchestrator examples"
heading: "How duplicates are avoided"
group: "Use"
order: 216
url: "/docs/orchestrator-examples#how-duplicates-are-avoided"
anchor: "how-duplicates-are-avoided"
terms: ["duplicates","avoided","intake","loop","avoids","duplicate","runs","event","receipt","keyed","source","action","mode","issue","gets","ever","relabelling","does","trigger","unless","encodes","transition","names","deterministic","hashes","host","refuses","second","start","unconfirmed","launches","never","resent","after","minutes","marked","manual","reconciliation","admitted","dispatch"]
hash: "edd465fc2e32ad38a7a50103f63899a7d7e1cf32e5d378906700e57651945a8c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/dispatch/orch-intake/source-markers/","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"src-intake-...","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"attempting","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"needs-reconcile","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"hev orchestrate reconcile intake --event ABC-12 --admitted","chunkId":"orchestrator-examples#how-duplicates-are-avoided"},{"kind":"code","literal":"--not-admitted","chunkId":"orchestrator-examples#how-duplicates-are-avoided"}]
sources: [{"chunkId":"orchestrator-examples#how-duplicates-are-avoided","url":"/docs/orchestrator-examples#how-duplicates-are-avoided","anchor":"how-duplicates-are-avoided"}]
---

The intake loop avoids duplicate runs with a per-event receipt keyed by source, event ID, action and mode, so each issue gets one run ever (relabelling does not re-trigger unless the event ID encodes the transition). Run names are deterministic hashes so the host refuses a second start; unconfirmed launches are never resent and, after 15 minutes, are marked for manual reconciliation as admitted or not.
