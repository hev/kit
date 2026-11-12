---
id: "orchestrator-examples#the-shape"
title: "Orchestrator examples"
heading: "The shape"
group: "Use"
order: 179
url: "/docs/orchestrator-examples#the-shape"
anchor: "the-shape"
terms: ["shape","every","intake","source","program","prints","events","rule","matches","event","status","order","template","action","renders","dispatches","through","admission","ships","tracker","plugin","write","built","watch","command","covers","simple","linear","polling","name","path","fields","agent","ready","same","three","parts","part","example","executable"]
hash: "9ddefa76d8311e940b133b9e455ddc4d982aafa004d073468eb3efb42ee790d6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-NAME","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"PATH","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev-linear","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fields.status","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"status: agent-ready","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev watch","chunkId":"orchestrator-examples#the-shape"}]
sources: [{"chunkId":"orchestrator-examples#the-shape","url":"/docs/orchestrator-examples#the-shape","anchor":"the-shape"}]
---

Every intake has a source program that prints events, a rule that matches event status to an order template, and an action that renders and dispatches through admission. kit ships no tracker plugin, so you write the source; the built-in watch command covers simple Linear status polling.
