---
id: "secrets#kits-own-reads"
title: "Secrets"
heading: "kit's own reads"
group: "Operate"
order: 253
url: "/docs/secrets#kits-own-reads"
anchor: "kits-own-reads"
terms: ["reads","secrets","layer","hour","native","gate","database","references","batched","through","same","cache","daemon","directly","prints","fallback","notice","stderr","answer","including","rate","limit","final","never","retries","hevlayer","azkv","board","serve","direct","1password","read","hevlayerapikeyref","older","hevlayerapikeyopref","still","works","hevd","every","other"]
hash: "2829942e325f8159403313561560f01dc1612f2cab19f12dd82bebf0e36b4a3a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"op://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"azkv://","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev board serve","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev","chunkId":"secrets#kits-own-reads"},{"kind":"code","literal":"hev secrets: notice: fallback to direct 1Password read","chunkId":"secrets#kits-own-reads"}]
sources: [{"chunkId":"secrets#kits-own-reads","url":"/docs/secrets#kits-own-reads","anchor":"kits-own-reads"}]
---

kit reads its own secrets, the hev layer API key (six-hour TTL) and the native gate's database references (one-hour TTL, batched), through the same cache. With no daemon each reads directly and prints a fallback notice on stderr, and any daemon answer including a rate limit is final so kit never retries it directly.
