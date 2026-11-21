---
id: "orchestrator-examples#the-spec"
title: "Orchestrator examples"
heading: "The spec"
group: "Use"
order: 304
url: "/docs/orchestrator-examples#the-spec"
anchor: "the-spec"
terms: ["spec","intake","loop","every","never","stops","until","lifetime","agent","model","runs","manages","issue","selector","dispatch","authority","linear","source","args","rule","renders","ready","status","mode","caps","pause","file","deterministic","admission","checks","event","fields","title","save","kind","orchestrator","name","goal","start","order"]
hash: "14023eeff657689f77b0568736231bdd63d65d8e62995bdeb2c0b1e537d779fd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"intake.md","chunkId":"orchestrator-examples#the-spec"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.ID","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.Fields.title","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.URL","chunkId":"orchestrator-examples#the-spec"}]
sources: [{"chunkId":"orchestrator-examples#the-spec","url":"/docs/orchestrator-examples#the-spec","anchor":"the-spec"}]
---

The intake spec: loop every 15m, never stops until lifetime 30d, agent off (no model runs), manages runs by issue selector, dispatch authority, a Linear source with args, a rule that renders fix.md for agent-ready status in dry-run mode, caps, and a pause file. Deterministic intake with admission checks.
