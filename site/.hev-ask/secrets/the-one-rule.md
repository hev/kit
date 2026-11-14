---
id: "secrets#the-one-rule"
title: "Secrets"
heading: "The one rule"
group: "Operate"
order: 316
url: "/docs/secrets#the-one-rule"
anchor: "the-one-rule"
terms: ["rule","always","read","secrets","through","commands","never","directly","scripts","profiles","agent","sessions","table","maps","needs","shell","variable","several","variables","program","environment","json","pipe","spend","cache","health","provenance","right","subcommand","values","only","travel","pipes","child","environments","defaults","five","minutes","hours","hour"]
hash: "087e4f9a8820f3a552ad291d20eae64e24ac33668a392db448be61a28e74879f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op read","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op inject","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op run","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"x=$(hev secrets read --label NAME --ttl 1h op://vault/item/field)","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"eval \"$(hev secrets env --ttl A=1h --ref A=op://… --ref B=op://…)\"","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets run --ref A=op://… --ttl A=1h -- program args","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets env --format json --ref A=op://…","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets status","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--no-quota","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--explain","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"env --format json-detail","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--ttl","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"0s","chunkId":"secrets#the-one-rule"}]
sources: [{"chunkId":"secrets#the-one-rule","url":"/docs/secrets#the-one-rule","anchor":"the-one-rule"}]
---

Always read secrets through the hev secrets commands, never the op CLI directly in scripts, profiles or agent sessions. A table maps needs (one shell variable, several variables, a program's environment, JSON on a pipe, spend, cache health, provenance) to the right subcommand; values only travel via pipes or child environments, and the TTL defaults to five minutes, max 24 hours, with an hour suiting most API keys.
