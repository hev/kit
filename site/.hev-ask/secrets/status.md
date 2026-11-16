---
id: "secrets#status"
title: "Secrets"
heading: "Status"
group: "Operate"
order: 269
url: "/docs/secrets#status"
anchor: "status"
terms: ["status","command","optional","json","output","shows","store","credential","type","reachability","hits","misses","requests","cooldown","plus","daemon","uptime","rate","limited","counts","read","hour","count","1password","quota","rows","checked","most","every","five","minutes","checking","skipped","vault","endpoint","secrets","programs","never","value","identifier"]
hash: "6ee94a231843b907d017aa3107bac0c78b1b2b561bcbfa47f352cae71de534c6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets status","chunkId":"secrets#status"},{"kind":"code","literal":"--json","chunkId":"secrets#status"},{"kind":"code","literal":"--no-quota","chunkId":"secrets#status"}]
sources: [{"chunkId":"secrets#status","url":"/docs/secrets#status","anchor":"status"}]
---

The status command, with optional JSON output, shows a row per store with credential type, reachability, hits, misses, requests and cooldown, plus daemon uptime, rate-limited counts, the read log's 24-hour count, and 1Password quota rows checked at most every five minutes. Quota checking can be skipped, and Key Vault has no quota endpoint.
