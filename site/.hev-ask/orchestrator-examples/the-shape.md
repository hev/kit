---
id: "orchestrator-examples#the-shape"
title: "Orchestrator examples"
heading: "The shape"
group: "Use"
order: 302
url: "/docs/orchestrator-examples#the-shape"
anchor: "the-shape"
terms: ["shape","intake","source","executable","name","answers","three","calls","describe","call","events","printing","json","lines","cursor","line","optional","ships","tracker","plugin","write","example","linear","supplies","rules","receipts","caps","pause","switch","parts","rule","matching","event","fields","action","rendering","order","path","status","agent"]
hash: "9ddefa76d8311e940b133b9e455ddc4d982aafa004d073468eb3efb42ee790d6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-NAME","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"PATH","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev-linear","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fields.status","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"status: agent-ready","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"fix.md","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"app","chunkId":"orchestrator-examples#the-shape"},{"kind":"code","literal":"hev watch","chunkId":"orchestrator-examples#the-shape"}]
sources: [{"chunkId":"orchestrator-examples#the-shape","url":"/docs/orchestrator-examples#the-shape","anchor":"the-shape"}]
---

An intake source is an executable hev-NAME that answers three calls: a describe call, an events call printing JSON lines then a cursor line, and an optional ack. kit ships no tracker plugin; you write the source (example for Linear) and kit supplies rules, receipts, caps and pause switch. Parts: source, rule matching event fields, action rendering an order.
