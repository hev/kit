---
id: "secrets#choose-a-provider"
title: "Secrets"
heading: "Choose a provider"
group: "Operate"
order: 249
url: "/docs/secrets#choose-a-provider"
anchor: "choose-a-provider"
terms: ["choose","provider","reference","scheme","selects","store","references","1password","through","service","account","azkv","azure","vault","latest","version","pinned","sovereign","cloud","hostnames","both","same","cache","batching","rate","limit","cooldown","behavior","read","whichever","already","holds","secret","host","needs","nothing","does","item","field","https"]
hash: "91ed3ce69e88aecadffc7452f03147b3b1fd1ef330460bc3a96b38a0c80479ed"
mode: "agent-primary"
facts: [{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"op","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"https://VAULT.vault.azure.net","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET/VERSION","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT.vault.usgovcloudapi.net/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"vault.azure.cn","chunkId":"secrets#choose-a-provider"}]
sources: [{"chunkId":"secrets#choose-a-provider","url":"/docs/secrets#choose-a-provider","anchor":"choose-a-provider"}]
---

The reference scheme selects the store: op references go to 1Password through a service account, azkv references go to Azure Key Vault (latest version, a pinned version, or sovereign cloud vault hostnames). Both get the same cache, TTL, batching, rate-limit and cooldown behavior and read log; use whichever store already holds the secret, and a host needs nothing for a store it does not use.
