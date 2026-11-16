---
id: "secrets#the-one-rule"
title: "Secrets"
heading: "The one rule"
group: "Operate"
order: 272
url: "/docs/secrets#the-one-rule"
anchor: "the-one-rule"
terms: ["rule","always","read","secrets","through","never","call","1password","inject","commands","directly","scripts","wrappers","profiles","agent","sessions","table","maps","needs","value","variable","several","current","shell","values","program","environment","json","pipe","host","spending","free","liveness","check","provenance","only","come","back","child","defaults"]
hash: "087e4f9a8820f3a552ad291d20eae64e24ac33668a392db448be61a28e74879f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op read","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op inject","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op run","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"x=$(hev secrets read --label NAME --ttl 1h op://vault/item/field)","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"eval \"$(hev secrets env --ttl A=1h --ref A=op://… --ref B=op://…)\"","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets run --ref A=op://… --ttl A=1h -- program args","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets env --format json --ref A=op://…","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets status","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--no-quota","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--explain","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"env --format json-detail","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--ttl","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"0s","chunkId":"secrets#the-one-rule"}]
sources: [{"chunkId":"secrets#the-one-rule","url":"/docs/secrets#the-one-rule","anchor":"the-one-rule"}]
---

Always read secrets through hev secrets and never call the 1Password CLI's read, inject or run commands directly in scripts, wrappers, profiles or agent sessions. A table maps needs to commands: one value into a variable, several into the current shell, values into a program's environment, JSON on a pipe, host spending, a free liveness check, and per-value provenance. Values only come back through a pipe or a child environment, and the TTL defaults to five minutes, maxes at 24 hours, and zero forces a fresh read; an hour suits most API keys.
