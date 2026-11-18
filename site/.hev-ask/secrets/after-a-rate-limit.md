---
id: "secrets#after-a-rate-limit"
title: "Secrets"
heading: "After a rate limit"
group: "Operate"
order: 308
url: "/docs/secrets#after-a-rate-limit"
anchor: "after-a-rate-limit"
terms: ["after","rate","limit","first","response","starts","host","wide","cooldown","recorded","disk","only","class","reset","time","until","every","process","gets","same","error","sends","nothing","while","daemon","keeps","serving","cached","values","cooldowns","store","mixed","request","fails","either","cooling","down","deleting","file","wastes"]
hash: "40d157da364871065e6a379ae73ab535c4f8acfbf91b264810b295a78c38f53a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/secrets/op-cooldown.json","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"hev secrets","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"Retry-After","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"azkv-cooldown.json","chunkId":"secrets#after-a-rate-limit"}]
sources: [{"chunkId":"secrets#after-a-rate-limit","url":"/docs/secrets#after-a-rate-limit","anchor":"after-a-rate-limit"}]
---

The first rate-limit response starts a host-wide cooldown recorded on disk with only the limit class and reset time; until reset every kit process gets the same error and sends nothing, while the daemon keeps serving cached values. Cooldowns are per store, a mixed request fails if either store is cooling down, and deleting the cooldown file only wastes more quota.
