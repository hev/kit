---
id: "secrets"
title: "Secrets"
heading: null
group: "Operate"
order: 361
url: "/docs/secrets"
anchor: null
terms: ["overview","memory","only","secrets","cache","daemon","named","store","agnostic","references","host","picks","1password","azure","vault","many","processes","share","request","protecting","shared","service","account","quotas","secret","name","stripe","item","field","azkv","callout","astro","codetabs","manager","read","through","local","cost","runs","inside"]
hash: "5efd5c56968c61873b0537f998ad737553c7db7efdcf009c759de4b288fa02db"
mode: "agent-primary"
facts: [{"kind":"code","literal":"secret://NAME","chunkId":"secrets"},{"kind":"code","literal":"secret://stripe-api-key","chunkId":"secrets"},{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets"},{"kind":"code","literal":"azkv://vault/secret","chunkId":"secrets"},{"kind":"code","literal":".env","chunkId":"secrets"},{"kind":"value","literal":"set-up-1password","chunkId":"secrets"},{"kind":"value","literal":"Callout.astro","chunkId":"secrets"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"secrets"}]
sources: [{"chunkId":"secrets","url":"/docs/secrets","anchor":null}]
---

Overview of kit's memory-only secrets cache in the daemon: secrets are named by store-agnostic references, each host picks 1Password or Azure Key Vault, and many processes share one request per TTL, protecting shared service-account quotas.
