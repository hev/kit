---
id: "secrets#the-one-rule"
title: "Secrets"
heading: "The one rule"
group: "Operate"
order: 261
url: "/docs/secrets#the-one-rule"
anchor: "the-one-rule"
terms: ["rule","always","read","secrets","through","never","call","inject","yourself","scripts","wrappers","profiles","agent","sessions","value","shell","variable","several","current","values","program","environment","json","pipe","report","spending","cache","health","explain","provenance","touch","disk","logs","process","arguments","defaults","five","minutes","maxes","hours"]
hash: "087e4f9a8820f3a552ad291d20eae64e24ac33668a392db448be61a28e74879f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op read","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op inject","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op run","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"x=$(hev secrets read --label NAME --ttl 1h op://vault/item/field)","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"eval \"$(hev secrets env --ttl A=1h --ref A=op://… --ref B=op://…)\"","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets run --ref A=op://… --ttl A=1h -- program args","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets env --format json --ref A=op://…","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets status","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--no-quota","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--explain","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"env --format json-detail","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--ttl","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"0s","chunkId":"secrets#the-one-rule"}]
sources: [{"chunkId":"secrets#the-one-rule","url":"/docs/secrets#the-one-rule","anchor":"the-one-rule"}]
---

Always read secrets through hev secrets and never call op read, inject or run yourself in scripts, wrappers, profiles or agent sessions. hev secrets can put one value in a shell variable, several in the current shell, values in a program's environment, JSON on a pipe, report spending and cache health, and explain provenance; values never touch disk, logs or process arguments. TTL defaults to five minutes, maxes at 24 hours, zero forces a fresh read, and an hour suits most API keys.
