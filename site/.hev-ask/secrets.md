---
id: "secrets"
title: "Secrets"
heading: null
group: "Operate"
order: 238
url: "/docs/secrets"
anchor: null
terms: ["runs","memory","only","secrets","cache","inside","hevd","daemon","1password","azure","vault","references","every","process","host","needing","same","value","costs","request","instead","protects","shared","daily","quota","service","account","avoids","throttling","both","stores","work","mixed","command","item","field","azkv","secret","callout","astro"]
hash: "223a76d80700a96dc835b9fa5b6590664616dd60f55c39c3b92fe9fe4e829f8f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets"},{"kind":"code","literal":"azkv://vault/secret","chunkId":"secrets"},{"kind":"code","literal":"op://","chunkId":"secrets"},{"kind":"code","literal":"azkv://","chunkId":"secrets"},{"kind":"value","literal":"set-up-1password","chunkId":"secrets"},{"kind":"value","literal":"Callout.astro","chunkId":"secrets"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"secrets"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"secrets"}]
sources: [{"chunkId":"secrets","url":"/docs/secrets","anchor":null}]
---

kit runs a memory-only secrets cache inside the hevd daemon for 1Password and Azure Key Vault references, so every process on a host needing the same value costs one request per TTL instead of one each. This protects the shared daily quota of a 1Password service account and avoids per-request vault throttling; both stores work the same way and can be mixed in one command.
