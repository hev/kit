---
id: "secrets"
title: "Secrets"
heading: null
group: "Operate"
order: 298
url: "/docs/secrets"
anchor: null
terms: ["runs","memory","only","secrets","cache","daemon","write","provider","neutral","named","references","host","configured","secret","manager","1password","azure","vault","decides","name","read","explicit","specific","forms","also","available","many","processes","needing","same","value","cost","request","matters","because","service","account","daily","quota","shared"]
hash: "5efd5c56968c61873b0537f998ad737553c7db7efdcf009c759de4b288fa02db"
mode: "agent-primary"
facts: [{"kind":"code","literal":"secret://NAME","chunkId":"secrets"},{"kind":"code","literal":"secret://stripe-api-key","chunkId":"secrets"},{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets"},{"kind":"code","literal":"azkv://vault/secret","chunkId":"secrets"},{"kind":"code","literal":".env","chunkId":"secrets"},{"kind":"value","literal":"set-up-1password","chunkId":"secrets"},{"kind":"value","literal":"Callout.astro","chunkId":"secrets"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"secrets"}]
sources: [{"chunkId":"secrets","url":"/docs/secrets","anchor":null}]
---

kit runs a memory-only secrets cache in the daemon. You write secrets as provider-neutral named references and each host's configured secret manager, 1Password or Azure Key Vault, decides where the name is read from, with explicit vault-specific forms also available. Many processes needing the same value cost one request per TTL, which matters because a 1Password service account's daily quota is shared across machines.
