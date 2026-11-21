---
id: "secrets#choose-a-provider"
title: "Secrets"
heading: "Choose a provider"
group: "Operate"
order: 372
url: "/docs/secrets#choose-a-provider"
anchor: "choose-a-provider"
terms: ["choose","provider","compares","stores","reference","scheme","picks","store","both","same","cache","limit","batching","attempt","rate","rule","cooldown","read","lists","secret","azkv","forms","name","vault","item","field","https","azure","version","usgovcloudapi","limits","host","manager","below","1password","through","service","account","path","latest"]
hash: "84cd5e77dda7ec092de4290f51b189bb8ab4417c8f3e7c590c7de07fdb26d0d1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"secret://NAME","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"op","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"https://VAULT.vault.azure.net","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET/VERSION","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT.vault.usgovcloudapi.net/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"vault.azure.cn","chunkId":"secrets#choose-a-provider"}]
sources: [{"chunkId":"secrets#choose-a-provider","url":"/docs/secrets#choose-a-provider","anchor":"choose-a-provider"}]
---

Compares the two stores: the reference scheme picks the store, and both get the same cache, TTL limit, batching, one-attempt rate-limit rule, cooldown and read log. Lists the secret://, op:// and azkv:// forms.
