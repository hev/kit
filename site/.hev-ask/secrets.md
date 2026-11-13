---
id: "secrets"
title: "Secrets"
heading: null
group: "Operate"
order: 244
url: "/docs/secrets"
anchor: null
terms: ["introduces","memory","only","secrets","cache","inside","hevd","lets","every","process","host","share","request","secret","stored","1password","azure","vault","protects","shared","daily","quota","service","account","avoids","throttling","everything","page","applies","both","stores","command","references","item","field","azkv","callout","astro","read","through"]
hash: "96e6f88b5f28f512c7a1ff75307a7bfe41c5eeee20d709ad277db8e97f554394"
mode: "agent-primary"
facts: [{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets"},{"kind":"code","literal":"azkv://vault/secret","chunkId":"secrets"},{"kind":"code","literal":"op://","chunkId":"secrets"},{"kind":"code","literal":"azkv://","chunkId":"secrets"},{"kind":"code","literal":".env","chunkId":"secrets"},{"kind":"value","literal":"set-up-1password","chunkId":"secrets"},{"kind":"value","literal":"Callout.astro","chunkId":"secrets"}]
sources: [{"chunkId":"secrets","url":"/docs/secrets","anchor":null}]
---

Introduces kit's memory-only secrets cache inside hevd, which lets every process on a host share one request per TTL for a secret stored in 1Password or Azure Key Vault. This protects the shared daily quota of a 1Password service account and avoids vault throttling. Everything on the page applies to both stores, and one command can mix references to both.
