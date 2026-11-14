---
id: "orchestrator-examples#the-shape"
title: "Orchestrator examples"
heading: "The shape"
group: "Use"
order: 229
url: "/docs/orchestrator-examples#the-shape"
anchor: "the-shape"
terms: ["shape","every","intake","three","parts","source","executable","path","emits","events","json","lines","rule","matching","event","status","order","template","action","renders","dispatches","through","admission","ships","tracker","plugin","program","speaking","protocol","works","plain","linear","needs","there","built","deterministic","watcher","name","fields","agent"]
hash: "9ddefa76d8311e940b133b9e455ddc4d982aafa004d073468eb3efb42ee790d6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-NAME","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"PATH","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev-linear","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fields.status","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"status: agent-ready","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev watch","chunkId":"orchestrator-examples#the-shape"}]
sources: [{"chunkId":"orchestrator-examples#the-shape","url":"/docs/orchestrator-examples#the-shape","anchor":"the-shape"}]
---

Every intake has three parts: a source executable on PATH that emits events as JSON lines, a rule matching an event's status to an order template, and an action that renders and dispatches through admission. kit ships no tracker plugin; any program speaking the source protocol works, and for plain Linear-status-to-order needs there is a built-in deterministic watcher.
