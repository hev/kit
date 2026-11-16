---
id: "secrets#choose-a-provider"
title: "Secrets"
heading: "Choose a provider"
group: "Operate"
order: 309
url: "/docs/secrets#choose-a-provider"
anchor: "choose-a-provider"
terms: ["choose","provider","reference","scheme","selects","store","neutral","form","uses","host","manager","explicit","forms","name","1password","vault","item","field","secret","optionally","pinned","version","sovereign","cloud","both","stores","share","same","cache","batching","cooldown","read","needs","nothing","does","azkv","https","azure","usgovcloudapi","picks"]
hash: "84cd5e77dda7ec092de4290f51b189bb8ab4417c8f3e7c590c7de07fdb26d0d1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"secret://NAME","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"op","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"https://VAULT.vault.azure.net","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET/VERSION","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT.vault.usgovcloudapi.net/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"vault.azure.cn","chunkId":"secrets#choose-a-provider"}]
sources: [{"chunkId":"secrets#choose-a-provider","url":"/docs/secrets#choose-a-provider","anchor":"choose-a-provider"}]
---

The reference's scheme selects the store: the neutral form uses the host's manager, and explicit forms name a 1Password vault, item and field or a Key Vault secret, optionally pinned to a version or in a sovereign cloud. Both stores share the same cache, TTL, batching, cooldown and read log, and a host needs nothing for a store it does not use.
