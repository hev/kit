---
id: "secrets#kits-own-reads"
title: "Secrets"
heading: "kit's own reads"
group: "Operate"
order: 263
url: "/docs/secrets#kits-own-reads"
anchor: "kits-own-reads"
terms: ["reads","secrets","through","same","cache","layer","reference","1password","vault","hour","native","gate","database","references","batch","without","daemon","read","directly","always","print","fallback","notice","stderr","naming","stores","reason","answer","including","rate","limit","final","hevlayer","azkv","board","serve","direct","azure","hevlayerapikeyref","older"]
hash: "4157fbd59237adca936e7dce95f91e37134f61e3b5108b15e20a288d15fa8e3b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"op://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"azkv://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev board serve","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev secrets: notice: fallback to direct","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"1Password read","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"Azure Key Vault read","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"1Password and Azure Key Vault read","chunkId":"secrets#kits-own-reads"}]
sources: [{"chunkId":"secrets#kits-own-reads","url":"/docs/secrets#kits-own-reads","anchor":"kits-own-reads"}]
---

kit reads its own secrets through the same cache: the hev layer API key reference (1Password or Key Vault) with a six-hour TTL, and the native gate's database references in one batch with a one-hour TTL. Without a daemon they read directly, but always print a fallback notice on stderr naming the stores and reason, and any daemon answer, including a rate limit, is final.
