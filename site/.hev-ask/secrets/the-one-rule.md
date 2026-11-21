---
id: "secrets#the-one-rule"
title: "Secrets"
heading: "The one rule"
group: "Operate"
order: 392
url: "/docs/secrets#the-one-rule"
anchor: "the-one-rule"
terms: ["rule","core","read","secrets","only","through","never","running","inject","scripts","wrappers","profiles","agent","sessions","table","maps","needs","value","several","values","command","right","usage","label","name","secret","eval","program","args","file","format","json","status","quota","ping","explain","detail","yourself","script","wrapper"]
hash: "af2bb5dacb942728061f41ad67174575b09334753399cd138912cf4b8bd4a9c5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op read","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op inject","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"op run","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"x=$(hev secrets read --label NAME --ttl 1h secret://name)","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"eval \"$(hev secrets env --ttl A=1h --ref A=secret://a --ref B=secret://b)\"","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets run --ref A=secret://a --ttl A=1h -- program args","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":".env","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets run --env-file .env -- program args","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets env --format json --ref A=secret://a","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets status","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--no-quota","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--explain","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"env --format json-detail","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"--ttl","chunkId":"secrets#the-one-rule"},{"kind":"code","literal":"0s","chunkId":"secrets#the-one-rule"}]
sources: [{"chunkId":"secrets#the-one-rule","url":"/docs/secrets#the-one-rule","anchor":"the-one-rule"}]
---

The core rule: read secrets only through hev secrets, never by running op read, inject or run in scripts, wrappers, profiles or agent sessions. A table maps needs (one value, several values, running a command, etc.) to the right usage.
