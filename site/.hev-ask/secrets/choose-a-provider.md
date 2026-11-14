---
id: "secrets#choose-a-provider"
title: "Secrets"
heading: "Choose a provider"
group: "Operate"
order: 303
url: "/docs/secrets#choose-a-provider"
anchor: "choose-a-provider"
terms: ["choose","provider","reference","scheme","selects","store","1password","service","account","azure","vault","latest","pinned","version","sovereign","cloud","references","whichever","already","holds","secret","host","needs","nothing","doesn","malformed","refused","item","field","azkv","https","usgovcloudapi","picks","both","stores","same","cache","limit","batching","attempt"]
hash: "91ed3ce69e88aecadffc7452f03147b3b1fd1ef330460bc3a96b38a0c80479ed"
mode: "agent-primary"
facts: [{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"op","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"https://VAULT.vault.azure.net","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT/SECRET/VERSION","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"azkv://VAULT.vault.usgovcloudapi.net/SECRET","chunkId":"secrets#choose-a-provider"},{"kind":"code","literal":"vault.azure.cn","chunkId":"secrets#choose-a-provider"}]
sources: [{"chunkId":"secrets#choose-a-provider","url":"/docs/secrets#choose-a-provider","anchor":"choose-a-provider"}]
---

The reference scheme selects the store: 1Password via a service account, or Azure Key Vault with latest, pinned-version or sovereign-cloud vault references. Use whichever store already holds the secret; a host needs nothing for a store it doesn't use, and malformed references are refused.
