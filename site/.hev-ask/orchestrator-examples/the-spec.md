---
id: "orchestrator-examples#the-spec"
title: "Orchestrator examples"
heading: "The spec"
group: "Use"
order: 241
url: "/docs/orchestrator-examples#the-spec"
anchor: "the-spec"
terms: ["spec","intake","orchestrator","deterministic","agent","turned","model","runs","rule","renders","order","template","newly","labelled","issue","checked","dispatch","admission","polls","schedule","indefinitely","within","thirty","lifetime","only","authority","small","budgets","tells","worker","stop","report","unclear","event","fields","title","save","kind","name","goal"]
hash: "14023eeff657689f77b0568736231bdd63d65d8e62995bdeb2c0b1e537d779fd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"intake.md","chunkId":"orchestrator-examples#the-spec"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.ID","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.Fields.title","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.URL","chunkId":"orchestrator-examples#the-spec"}]
sources: [{"chunkId":"orchestrator-examples#the-spec","url":"/docs/orchestrator-examples#the-spec","anchor":"the-spec"}]
---

The intake orchestrator spec is deterministic: with its agent turned off no model runs, and a rule renders an order template for each newly labelled issue, checked by dispatch admission. It polls on a schedule indefinitely within a thirty-day lifetime, has dispatch-only authority and small budgets, and its template tells the worker to stop and report if the issue is unclear or too big for one PR.
