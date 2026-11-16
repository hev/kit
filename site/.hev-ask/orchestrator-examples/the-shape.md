---
id: "orchestrator-examples#the-shape"
title: "Orchestrator examples"
heading: "The shape"
group: "Use"
order: 239
url: "/docs/orchestrator-examples#the-shape"
anchor: "the-shape"
terms: ["shape","every","intake","three","parts","source","executable","prints","events","json","lines","rule","matches","event","status","order","template","action","renders","dispatches","through","admission","ships","tracker","plugin","write","provides","scheduling","rules","receipts","caps","pause","though","built","poller","exists","simple","linear","case","name"]
hash: "9ddefa76d8311e940b133b9e455ddc4d982aafa004d073468eb3efb42ee790d6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-NAME","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"PATH","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev-linear","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fields.status","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"status: agent-ready","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev watch","chunkId":"orchestrator-examples#the-shape"}]
sources: [{"chunkId":"orchestrator-examples#the-shape","url":"/docs/orchestrator-examples#the-shape","anchor":"the-shape"}]
---

Every intake has three parts: a source executable that prints events as JSON lines, a rule that matches an event's status to an order template, and an action that renders and dispatches the order through admission. kit ships no tracker plugin; you write the source and kit provides scheduling, rules, receipts, caps and pause, though a built-in poller exists for the simple Linear-status case.
