---
id: "secrets#choose-a-provider"
title: "Secrets"
heading: "Choose a provider"
group: "Operate"
order: 259
url: "/docs/secrets#choose-a-provider"
anchor: "choose-a-provider"
terms: ["choose","provider","reference","scheme","selects","store","1password","service","account","azure","vault","latest","version","pinned","sovereign","cloud","whose","suffix","sets","sign","host","both","stores","same","cache","limit","batching","rate","rule","cooldown","read","needs","setup","does","references","ports","queries","extra","path","segments"]
hash: "91ed3ce69e88aecadffc7452f03147b3b1fd1ef330460bc3a96b38a0c80479ed"
mode: "agent-primary"
facts: [{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"op","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"https://VAULT.vault.azure.net","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET/VERSION","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT.vault.usgovcloudapi.net/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"vault.azure.cn","chunkId":"secrets#choose-a-provider"}]
sources: [{"chunkId":"secrets#choose-a-provider","url":"/docs/secrets#choose-a-provider","anchor":"choose-a-provider"}]
---

The reference scheme selects the store: 1Password via a service account, or Azure Key Vault with the latest version, a pinned version, or a sovereign-cloud vault whose suffix sets the sign-in host. Both stores get the same cache, TTL limit, batching, rate-limit rule, cooldown and read log, and a host needs no setup for a store it does not use. References with ports, queries, extra path segments or trailing slashes are refused.
