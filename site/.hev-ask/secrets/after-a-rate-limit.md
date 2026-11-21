---
id: "secrets#after-a-rate-limit"
title: "Secrets"
heading: "After a rate limit"
group: "Operate"
order: 371
url: "/docs/secrets#after-a-rate-limit"
anchor: "after-a-rate-limit"
terms: ["after","rate","limit","first","throttle","answer","starts","store","host","wide","cooldown","recorded","small","file","every","process","stops","sending","until","reset","while","daemon","keeps","serving","cached","values","vault","throttling","retry","handled","attempt","only","secrets","json","azkv","whole","class","time","nothing","else"]
hash: "40d157da364871065e6a379ae73ab535c4f8acfbf91b264810b295a78c38f53a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/secrets/op-cooldown.json","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"hev secrets","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"Retry-After","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"azkv-cooldown.json","chunkId":"secrets#after-a-rate-limit"}]
sources: [{"chunkId":"secrets#after-a-rate-limit","url":"/docs/secrets#after-a-rate-limit","anchor":"after-a-rate-limit"}]
---

After a rate limit, the first throttle answer starts a per-store, host-wide cooldown recorded in a small file; every kit process stops sending until reset while the daemon keeps serving cached values. Key Vault throttling (429 with Retry-After) is handled with one attempt only.
