---
id: "secrets#status"
title: "Secrets"
heading: "Status"
group: "Operate"
order: 326
url: "/docs/secrets#status"
anchor: "status"
terms: ["status","secrets","reports","host","secret","manager","store","credential","type","reachability","hits","misses","requests","cooldown","daemon","uptime","real","rate","limited","request","counts","read","daily","count","1password","quota","rows","refreshed","only","explicit","meter","vault","endpoint","json","refresh","programs","shows","there","none","never"]
hash: "49c04500d4557b3c6310d77fc90624a46a476c7580f04f1981db1bb528dd960a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets status","chunkId":"secrets#status"},{"kind":"code","literal":"--json","chunkId":"secrets#status"},{"kind":"code","literal":"secret://","chunkId":"secrets#status"},{"kind":"code","literal":"manager","chunkId":"secrets#status"},{"kind":"code","literal":"--refresh-quota","chunkId":"secrets#status"}]
sources: [{"chunkId":"secrets#status","url":"/docs/secrets#status","anchor":"status"}]
---

Secrets status reports the host's secret manager and where it is set, one row per store with credential type, reachability, hits, misses, requests and cooldown, daemon uptime, real and rate-limited request counts, and the read log's daily count. 1Password quota rows are refreshed only on explicit request or on the meter host, and Key Vault has no quota endpoint.
