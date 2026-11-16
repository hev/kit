---
id: "orchestrator-examples#the-shape"
title: "Orchestrator examples"
heading: "The shape"
group: "Use"
order: 185
url: "/docs/orchestrator-examples#the-shape"
anchor: "the-shape"
terms: ["shape","every","intake","three","parts","source","executable","prints","events","json","lines","rule","matching","event","status","order","template","action","renders","dispatches","through","admission","only","knows","tracker","ships","plugin","built","watch","exists","plain","linear","statuses","name","path","fields","agent","ready","same","part"]
hash: "9ddefa76d8311e940b133b9e455ddc4d982aafa004d073468eb3efb42ee790d6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-NAME","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"PATH","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev-linear","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fields.status","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"status: agent-ready","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev watch","chunkId":"orchestrator-examples#the-shape"}]
sources: [{"chunkId":"orchestrator-examples#the-shape","url":"/docs/orchestrator-examples#the-shape","anchor":"the-shape"}]
---

Every intake has three parts: a source executable that prints events as JSON lines, a rule matching an event's status to an order template, and an action that renders and dispatches through admission. Only the source knows the tracker, and kit ships no tracker plugin. A built-in watch exists for plain Linear statuses.
