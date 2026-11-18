---
id: "secrets#kits-own-reads"
title: "Secrets"
heading: "kit's own reads"
group: "Operate"
order: 315
url: "/docs/secrets#kits-own-reads"
anchor: "kits-own-reads"
terms: ["reads","secrets","through","same","cache","layer","reference","hour","native","gate","database","references","batch","direct","read","without","daemon","never","silent","printing","fallback","notice","answer","including","rate","limit","final","hevlayer","secret","azkv","board","serve","1password","azure","vault","hevlayerapikeyref","older","hevlayerapikeyopref","still","works"]
hash: "81a92946b6d9a60d7febc51b43be9d0fc89775dabb76170aba28fb0cbdd349f7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"secret://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"op://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"azkv://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev board serve","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev secrets: notice: fallback to direct","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"1Password read","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"Azure Key Vault read","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"1Password and Azure Key Vault read","chunkId":"secrets#kits-own-reads"}]
sources: [{"chunkId":"secrets#kits-own-reads","url":"/docs/secrets#kits-own-reads","anchor":"kits-own-reads"}]
---

kit reads its own secrets through the same cache: the hev layer key by reference with a six-hour TTL, and the native gate's database references in one batch with a one-hour TTL. A direct read without the daemon is never silent, printing a fallback notice, and any answer from the daemon, including a rate limit, is final.
