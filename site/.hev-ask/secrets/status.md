---
id: "secrets#status"
title: "Secrets"
heading: "Status"
group: "Operate"
order: 258
url: "/docs/secrets#status"
anchor: "status"
terms: ["status","secrets","command","shows","store","credential","type","last","reachability","hits","misses","requests","cooldown","plus","daemon","uptime","rate","limited","counts","active","read","hour","count","1password","quota","rows","checked","most","every","five","minutes","flag","skips","check","vault","endpoint","json","programs","never","value"]
hash: "6ee94a231843b907d017aa3107bac0c78b1b2b561bcbfa47f352cae71de534c6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets status","chunkId":"secrets#status"},{"kind":"code","literal":"--json","chunkId":"secrets#status"},{"kind":"code","literal":"--no-quota","chunkId":"secrets#status"}]
sources: [{"chunkId":"secrets#status","url":"/docs/secrets#status","anchor":"status"}]
---

The secrets status command shows one row per store with credential type, last reachability, hits, misses, requests and cooldown, plus daemon uptime, rate-limited counts, the active cooldown, the read log's 24-hour count, and 1Password quota rows checked at most every five minutes. A flag skips the quota check, and Key Vault has no quota endpoint.
