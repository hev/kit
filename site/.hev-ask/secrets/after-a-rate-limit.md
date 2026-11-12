---
id: "secrets#after-a-rate-limit"
title: "Secrets"
heading: "After a rate limit"
group: "Operate"
order: 248
url: "/docs/secrets#after-a-rate-limit"
anchor: "after-a-rate-limit"
terms: ["after","rate","limit","first","response","starts","host","wide","cooldown","recorded","file","until","reset","every","process","gets","same","error","without","sending","requests","while","hevd","keeps","serving","cached","values","cooldowns","store","vault","retry","honoured","separately","request","mixing","both","stores","fails","either","cooling"]
hash: "40d157da364871065e6a379ae73ab535c4f8acfbf91b264810b295a78c38f53a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/secrets/op-cooldown.json","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"hev secrets","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"Retry-After","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"azkv-cooldown.json","chunkId":"secrets#after-a-rate-limit"}]
sources: [{"chunkId":"secrets#after-a-rate-limit","url":"/docs/secrets#after-a-rate-limit","anchor":"after-a-rate-limit"}]
---

The first rate-limit response starts a host-wide cooldown recorded in a cooldown file, and until the reset every kit process on the host gets the same error without sending requests, while hevd keeps serving cached values. Cooldowns are per store (Key Vault 429 with Retry-After is honoured separately), a request mixing both stores fails if either is cooling down, and you should not delete the cooldown file to unstick a host.
