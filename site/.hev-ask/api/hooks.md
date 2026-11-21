---
id: "api/hooks"
title: "Hooks API"
heading: null
group: "API"
order: 32
url: "/docs/api/hooks"
anchor: null
terms: ["hooks","hosted","side","register","webhooks","harness","events","read","event","delivery","history","send","test","deliveries","verify","signed","payloads","local","normalize","hook","json","routes","rolling","club","accounts","train","claude","codex","name","path","jsonl","post","patch","delete","warnings","hookevent","blockinghookevent","callout","astro","codetabs"]
hash: "7482334cccc3bbd5f39a7d862d2165d9e0c3d8e7e97ea1c1dc2207b913e33cd4"
mode: "source-primary"
facts: [{"kind":"code","literal":"/v1/hooks","chunkId":"api/hooks"},{"kind":"code","literal":"hev hook EVENT --harness claude|codex","chunkId":"api/hooks"},{"kind":"code","literal":"hev.hook.event/v1","chunkId":"api/hooks"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"api/hooks"},{"kind":"code","literal":"PATH","chunkId":"api/hooks"},{"kind":"code","literal":"~/.hev/hooks/events.jsonl","chunkId":"api/hooks"},{"kind":"code","literal":"GET","chunkId":"api/hooks"},{"kind":"code","literal":"POST /v1/hooks","chunkId":"api/hooks"},{"kind":"code","literal":"PATCH","chunkId":"api/hooks"},{"kind":"code","literal":"DELETE /v1/hooks/{id}","chunkId":"api/hooks"},{"kind":"code","literal":"GET /v1/hooks/events","chunkId":"api/hooks"},{"kind":"code","literal":"POST /v1/hooks/events","chunkId":"api/hooks"},{"kind":"code","literal":"hev hook","chunkId":"api/hooks"},{"kind":"code","literal":"GET /v1/hooks/{id}/deliveries","chunkId":"api/hooks"},{"kind":"code","literal":"~/.hev/hooks/warnings.log","chunkId":"api/hooks"},{"kind":"code","literal":"POST /v1/hooks/{id}/test","chunkId":"api/hooks"},{"kind":"code","literal":"hookEvent","chunkId":"api/hooks"},{"kind":"code","literal":"blockingHookEvent","chunkId":"api/hooks"},{"kind":"value","literal":"Callout.astro","chunkId":"api/hooks"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"api/hooks"},{"kind":"value","literal":"LinkGrid.astro","chunkId":"api/hooks"},{"kind":"value","literal":"0.7","chunkId":"api/hooks"}]
sources: [{"chunkId":"api/hooks","url":"/docs/api/hooks","anchor":null}]
---

The hooks API is the hosted side of hooks: register webhooks for harness and kit events, read the event log and delivery history, send test deliveries and verify signed payloads. In OSS hooks are local and normalize harness hook JSON. The hosted hooks routes are rolling out to Club Kit and Pro Kit accounts on the 0.7 train.
