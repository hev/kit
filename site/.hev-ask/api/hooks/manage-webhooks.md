---
id: "api/hooks#manage-webhooks"
title: "Hooks API"
heading: "Manage webhooks"
group: "API"
order: 37
url: "/docs/api/hooks#manage-webhooks"
anchor: "manage-webhooks"
terms: ["manage","webhooks","list","read","update","delete","reading","hook","shows","hour","health","healthy","failing","idle","disabled","last","delivery","consecutive","failures","includes","secret","rotation","test","deliveries","hooks","enabled","blocking","event","tool","state","success","rate","failed","open","pending","retries","patch","true","reason","rotate"]
hash: "a3fae54bd2f4111323448de1493e259acbb2150113a5584ed160c52620b8ff6e"
mode: "source-primary"
facts: [{"kind":"code","literal":"GET /v1/hooks","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"enabled","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"blocking","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"event","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"?event=pre-tool","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"GET /v1/hooks/{id}","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"health","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"state","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"healthy","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"failing","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"idle","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"disabled","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"consecutive_failures","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"deliveries_24h","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"success_rate_24h","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"failed_open_24h","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"pending_retries","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"PATCH /v1/hooks/{id}","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"enabled: true","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"disabled_reason","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"rotate_secret: true","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"secret","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"url","chunkId":"api/hooks#manage-webhooks"},{"kind":"code","literal":"DELETE /v1/hooks/{id}","chunkId":"api/hooks#manage-webhooks"}]
sources: [{"chunkId":"api/hooks#manage-webhooks","url":"/docs/api/hooks#manage-webhooks","anchor":"manage-webhooks"}]
---

List, read, update and delete webhooks; reading a hook shows 24-hour health (healthy, failing, idle, disabled), last delivery and consecutive failures. Includes secret rotation and test deliveries.
