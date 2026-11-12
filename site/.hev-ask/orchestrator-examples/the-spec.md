---
id: "orchestrator-examples#the-spec"
title: "Orchestrator examples"
heading: "The spec"
group: "Use"
order: 181
url: "/docs/orchestrator-examples#the-spec"
anchor: "the-spec"
terms: ["spec","intake","loops","every","minutes","indefinitely","days","model","scoped","issue","prefix","repo","only","dispatch","authority","linear","source","rule","mapping","label","order","template","plus","budget","pause","file","renders","title","tells","worker","stop","unclear","event","fields","save","kind","orchestrator","name","goal","start"]
hash: "14023eeff657689f77b0568736231bdd63d65d8e62995bdeb2c0b1e537d779fd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"intake.md","chunkId":"orchestrator-examples#the-spec"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.ID","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.Fields.title","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.URL","chunkId":"orchestrator-examples#the-spec"}]
sources: [{"chunkId":"orchestrator-examples#the-spec","url":"/docs/orchestrator-examples#the-spec","anchor":"the-spec"}]
---

The intake spec loops every 15 minutes indefinitely up to 30 days with no model, scoped by issue prefix and repo, with only dispatch authority, a Linear source and one dry-run rule mapping a label to an order template, plus budget and pause file. The order template renders the issue key, title and URL and tells the worker to stop if the issue is unclear or too big.
