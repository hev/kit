---
id: "secrets#kits-own-reads"
title: "Secrets"
heading: "kit's own reads"
group: "Operate"
order: 307
url: "/docs/secrets#kits-own-reads"
anchor: "kits-own-reads"
terms: ["reads","secrets","through","same","cache","layer","reference","hour","native","gate","database","references","hourly","batch","without","daemon","directly","always","prints","fallback","notice","answer","including","rate","limit","final","hevlayer","azkv","board","serve","direct","1password","read","hevlayerapikeyref","older","hevlayerapikeyopref","still","works","hevd","every"]
hash: "2829942e325f8159403313561560f01dc1612f2cab19f12dd82bebf0e36b4a3a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"op://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"azkv://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev board serve","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev secrets: notice: fallback to direct 1Password read","chunkId":"secrets#kits-own-reads"}]
sources: [{"chunkId":"secrets#kits-own-reads","url":"/docs/secrets#kits-own-reads","anchor":"kits-own-reads"}]
---

kit reads its own secrets through the same cache: the hev layer API key reference with a six-hour TTL, and the native gate's database references in one hourly batch. Without a daemon each reads directly but always prints a fallback notice, and any daemon answer, including a rate limit, is final.
