---
id: "secrets#status"
title: "Secrets"
heading: "Status"
group: "Operate"
order: 313
url: "/docs/secrets#status"
anchor: "status"
terms: ["status","secrets","shows","store","credential","type","reachability","hits","misses","requests","cooldown","plus","daemon","uptime","rate","limited","counts","read","daily","count","1password","quota","rows","checked","most","every","five","minutes","skippable","vault","endpoint","json","programs","never","value","identifier","last","real","request","unconfigured"]
hash: "6ee94a231843b907d017aa3107bac0c78b1b2b561bcbfa47f352cae71de534c6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets status","chunkId":"secrets#status"},{"kind":"code","literal":"--json","chunkId":"secrets#status"},{"kind":"code","literal":"--no-quota","chunkId":"secrets#status"}]
sources: [{"chunkId":"secrets#status","url":"/docs/secrets#status","anchor":"status"}]
---

Secrets status shows, per store, the credential type, reachability, hits, misses, requests and cooldown, plus daemon uptime, rate-limited counts, the read log's daily count and 1Password quota rows (checked at most every five minutes and skippable). Key Vault has no quota endpoint.
