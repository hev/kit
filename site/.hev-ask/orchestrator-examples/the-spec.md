---
id: "orchestrator-examples#the-spec"
title: "Orchestrator examples"
heading: "The spec"
group: "Use"
order: 187
url: "/docs/orchestrator-examples#the-spec"
anchor: "the-spec"
terms: ["spec","intake","loop","polls","every","minutes","never","stops","lasts","days","runs","model","manages","issue","selector","repo","dispatch","authority","only","uses","plugin","source","team","rule","mode","small","caps","pause","file","event","fields","title","save","kind","orchestrator","name","goal","start","order","open"]
hash: "14023eeff657689f77b0568736231bdd63d65d8e62995bdeb2c0b1e537d779fd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"intake.md","chunkId":"orchestrator-examples#the-spec"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.ID","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.Fields.title","chunkId":"orchestrator-examples#the-spec"},{"kind":"value","literal":"Event.URL","chunkId":"orchestrator-examples#the-spec"}]
sources: [{"chunkId":"orchestrator-examples#the-spec","url":"/docs/orchestrator-examples#the-spec","anchor":"the-spec"}]
---

The intake spec is a loop that polls every 15 minutes, never stops on its own, lasts up to 30 days, runs no model, manages runs by an issue selector in one repo, and has dispatch authority only. It uses a plugin source for a team and one rule in dry-run mode, with small caps and a pause file.
