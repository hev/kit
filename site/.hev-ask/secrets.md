---
id: "secrets"
title: "Secrets"
heading: null
group: "Operate"
order: 288
url: "/docs/secrets"
anchor: null
terms: ["runs","memory","only","secrets","cache","inside","daemon","every","process","host","needing","same","1password","azure","vault","value","costs","request","matters","because","service","account","daily","quota","shared","across","machines","vaults","throttle","both","stores","work","mixed","command","item","field","azkv","secret","callout","astro"]
hash: "96e6f88b5f28f512c7a1ff75307a7bfe41c5eeee20d709ad277db8e97f554394"
mode: "agent-primary"
facts: [{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets"},{"kind":"code","literal":"azkv://vault/secret","chunkId":"secrets"},{"kind":"code","literal":"op://","chunkId":"secrets"},{"kind":"code","literal":"azkv://","chunkId":"secrets"},{"kind":"code","literal":".env","chunkId":"secrets"},{"kind":"value","literal":"set-up-1password","chunkId":"secrets"},{"kind":"value","literal":"Callout.astro","chunkId":"secrets"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"secrets"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"secrets"}]
sources: [{"chunkId":"secrets","url":"/docs/secrets","anchor":null}]
---

kit runs a memory-only secrets cache inside the daemon so every process on a host needing the same 1Password or Azure Key Vault value costs one request per TTL. This matters because a 1Password service account's daily quota is shared across machines and vaults throttle per request; both stores work the same way and can be mixed in one command.
