---
id: "secrets#the-one-rule"
title: "Secrets"
heading: "The one rule"
group: "Operate"
order: 329
url: "/docs/secrets#the-one-rule"
anchor: "the-one-rule"
terms: ["rule","read","secrets","only","through","commands","never","call","1password","directly","scripts","wrappers","profiles","agent","sessions","table","maps","needs","value","several","shell","variables","program","environment","file","references","json","spending","cache","liveness","provenance","right","command","values","return","pipe","child","defaults","five","minutes"]
hash: "af2bb5dacb942728061f41ad67174575b09334753399cd138912cf4b8bd4a9c5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op read","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op inject","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op run","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"x=$(hev secrets read --label NAME --ttl 1h secret://name)","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"eval \"$(hev secrets env --ttl A=1h --ref A=secret://a --ref B=secret://b)\"","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets run --ref A=secret://a --ttl A=1h -- program args","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":".env","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets run --env-file .env -- program args","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets env --format json --ref A=secret://a","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets status","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--no-quota","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--explain","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"env --format json-detail","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--ttl","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"0s","chunkId":"secrets#the-one-rule"}]
sources: [{"chunkId":"secrets#the-one-rule","url":"/docs/secrets#the-one-rule","anchor":"the-one-rule"}]
---

The one rule is to read secrets only through hev's secrets commands and never call the 1Password CLI directly in scripts, wrappers, profiles or agent sessions. A table maps needs (one value, several shell variables, a program's environment, an env file of references, JSON, spending, cache liveness, provenance) to the right command; values return only via a pipe or child environment, and TTL defaults to five minutes with a 24-hour maximum.
