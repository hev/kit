---
id: "secrets#after-a-rate-limit"
title: "Secrets"
heading: "After a rate limit"
group: "Operate"
order: 258
url: "/docs/secrets#after-a-rate-limit"
anchor: "after-a-rate-limit"
terms: ["after","rate","limit","first","response","starts","host","wide","cooldown","recorded","small","file","beside","read","until","reset","every","process","gets","same","error","without","sending","anything","while","hevd","keeps","serving","cached","values","cooldowns","store","vault","responses","honour","retry","their","request","mixing","both"]
hash: "40d157da364871065e6a379ae73ab535c4f8acfbf91b264810b295a78c38f53a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/secrets/op-cooldown.json","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"hev secrets","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"Retry-After","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"azkv-cooldown.json","chunkId":"secrets#after-a-rate-limit"}]
sources: [{"chunkId":"secrets#after-a-rate-limit","url":"/docs/secrets#after-a-rate-limit","anchor":"after-a-rate-limit"}]
---

The first rate-limit response starts a host-wide cooldown recorded in a small file beside the read log, and until the reset every kit process on the host gets the same error without sending anything, while hevd keeps serving cached values. Cooldowns are per store: Key Vault 429 responses honour Retry-After in their own file, and a request mixing both stores fails if either is cooling down. Do not delete the cooldown file to unstick a host; malformed files or ones claiming more than 24 hours are ignored.
