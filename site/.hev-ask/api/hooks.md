---
id: "api/hooks"
title: "Hooks API"
heading: null
group: "API"
order: 32
url: "/docs/api/hooks"
anchor: null
terms: ["hooks","hosted","side","register","webhooks","harness","events","read","event","delivery","history","send","test","deliveries","verify","signed","payloads","local","normalize","hook","json","claude","codex","name","path","jsonl","post","patch","delete","warnings","hookevent","blockinghookevent","callout","astro","codetabs","linkgrid","meta","normalizes","runs","handlers"]
hash: "9c7bd52f047705afbca98279b5d287a5351c20d399d86c4da38fe17a92d18f78"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev hook EVENT --harness claude|codex","chunkId":"api/hooks"},{"kind":"code","literal":"hev.hook.event/v1","chunkId":"api/hooks"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"api/hooks"},{"kind":"code","literal":"PATH","chunkId":"api/hooks"},{"kind":"code","literal":"~/.hev/hooks/events.jsonl","chunkId":"api/hooks"},{"kind":"code","literal":"GET","chunkId":"api/hooks"},{"kind":"code","literal":"POST /v1/hooks","chunkId":"api/hooks"},{"kind":"code","literal":"PATCH","chunkId":"api/hooks"},{"kind":"code","literal":"DELETE /v1/hooks/{id}","chunkId":"api/hooks"},{"kind":"code","literal":"GET /v1/hooks/events","chunkId":"api/hooks"},{"kind":"code","literal":"POST /v1/hooks/events","chunkId":"api/hooks"},{"kind":"code","literal":"hev hook","chunkId":"api/hooks"},{"kind":"code","literal":"GET /v1/hooks/{id}/deliveries","chunkId":"api/hooks"},{"kind":"code","literal":"~/.hev/hooks/warnings.log","chunkId":"api/hooks"},{"kind":"code","literal":"POST /v1/hooks/{id}/test","chunkId":"api/hooks"},{"kind":"code","literal":"hookEvent","chunkId":"api/hooks"},{"kind":"code","literal":"blockingHookEvent","chunkId":"api/hooks"},{"kind":"value","literal":"Callout.astro","chunkId":"api/hooks"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"api/hooks"},{"kind":"value","literal":"LinkGrid.astro","chunkId":"api/hooks"}]
sources: [{"chunkId":"api/hooks","url":"/docs/api/hooks","anchor":null}]
---

The hooks API is the hosted side of hooks: register webhooks for harness and kit events, read the event log and delivery history, send test deliveries and verify signed payloads. In OSS hooks are local and normalize harness hook JSON.
