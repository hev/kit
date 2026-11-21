---
id: "notifications#the-notice"
title: "Notifications"
heading: "The notice"
group: "Use"
order: 283
url: "/docs/notifications#the-notice"
anchor: "the-notice"
terms: ["notice","json","schema","attention","event","time","source","name","host","state","text","links","thread","requestedby","addressee","change","delivered","requested","outbox","notify","client","true","att5f0c9e1b2a7d4c3e8f6a1b2c","2026","17t03","needs","buildbox","review","decision","postgres","sqlite","https","github","example","project","pull","dispatch","show","person","user123"]
hash: "76b9c274785a4c1a579cef2a856282deb7b4673bb6df7ee2c890312e0e9240fa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev.attention.event/v1","chunkId":"notifications#the-notice"},{"kind":"code","literal":"requested_by","chunkId":"notifications#the-notice"},{"kind":"code","literal":"addressee","chunkId":"notifications#the-notice"},{"kind":"code","literal":"~/.hev/attention/outbox/","chunkId":"notifications#the-notice"},{"kind":"code","literal":"[notify] client = true","chunkId":"notifications#the-notice"},{"kind":"code","literal":"client: true","chunkId":"notifications#the-notice"}]
sources: [{"chunkId":"notifications#the-notice","url":"/docs/notifications#the-notice","anchor":"the-notice"}]
---

Each notice is JSON with schema hev.attention.event/v1: id, time, event, source, name, host, state, text, links (PR, run), thread, requestedby and addressee. requestedby and addressee don't change where the notice is delivered.
