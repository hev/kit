---
id: "secrets#after-a-rate-limit"
title: "Secrets"
heading: "After a rate limit"
group: "Operate"
order: 302
url: "/docs/secrets#after-a-rate-limit"
anchor: "after-a-rate-limit"
terms: ["after","rate","limit","first","response","puts","whole","host","store","cooldown","recorded","file","beside","read","every","process","gets","same","error","sends","nothing","until","reset","daemon","stays","serving","cached","values","vault","429s","honor","retry","stores","cooldowns","independent","mixed","requests","fail","either","cooling"]
hash: "40d157da364871065e6a379ae73ab535c4f8acfbf91b264810b295a78c38f53a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/secrets/op-cooldown.json","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"hev secrets","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"Retry-After","chunkId":"secrets#after-a-rate-limit"},{"kind":"code","literal":"azkv-cooldown.json","chunkId":"secrets#after-a-rate-limit"}]
sources: [{"chunkId":"secrets#after-a-rate-limit","url":"/docs/secrets#after-a-rate-limit","anchor":"after-a-rate-limit"}]
---

The first rate-limit response puts the whole host into a per-store cooldown recorded in a file beside the read log, so every kit process gets the same error and sends nothing until reset; the daemon stays up serving cached values. Key Vault 429s honor Retry-After; the stores' cooldowns are independent, mixed requests fail if either is cooling down, and you should not delete the cooldown file to unstick a host.
