---
id: "secrets#kits-own-reads"
title: "Secrets"
heading: "kit's own reads"
group: "Operate"
order: 378
url: "/docs/secrets#kits-own-reads"
anchor: "kits-own-reads"
terms: ["reads","secrets","such","layer","reference","through","same","cache","hour","hevd","board","serve","other","processes","share","read","hevlayer","secret","azkv","notice","fallback","direct","1password","azure","vault","hevlayerapikeyref","older","hevlayerapikeyopref","still","works","daemon","every","command","needs","native","gate","database","references","resolved","batch"]
hash: "81a92946b6d9a60d7febc51b43be9d0fc89775dabb76170aba28fb0cbdd349f7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"secret://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"op://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"azkv://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev board serve","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev secrets: notice: fallback to direct","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"1Password read","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"Azure Key Vault read","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"1Password and Azure Key Vault read","chunkId":"secrets#kits-own-reads"}]
sources: [{"chunkId":"secrets#kits-own-reads","url":"/docs/secrets#kits-own-reads","anchor":"kits-own-reads"}]
---

kit reads its own secrets, such as the hev layer key reference, through the same cache with a six-hour TTL, so hevd, board serve and other processes share one read.
