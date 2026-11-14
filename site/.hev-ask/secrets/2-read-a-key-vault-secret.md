---
id: "secrets#2-read-a-key-vault-secret"
title: "Secrets"
heading: "2. Read a Key Vault secret"
group: "Operate"
order: 294
url: "/docs/secrets#2-read-a-key-vault-secret"
anchor: "2-read-a-key-vault-secret"
terms: ["read","vault","secret","ping","cache","shell","variable","explain","output","checking","empty","without","printing","should","show","came","daemon","exit","means","throttled","retry","http","errors","reported","printed","secrets","password","label","azkv","dbpassword","azure","through","first","print","names","give","never","echo","value","only"]
hash: "6e261065788dd91f01a5496d3514155d9082e333de00eb7e7ea233585999b9af"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets ping\nDB_PASSWORD=$(hev secrets read --label DB_PASSWORD --ttl 1h --explain azkv://VAULT/SECRET)","chunkId":"secrets#2-read-a-key-vault-secret"}]
sources: [{"chunkId":"secrets#2-read-a-key-vault-secret","url":"/docs/secrets#2-read-a-key-vault-secret","anchor":"2-read-a-key-vault-secret"}]
---

Ping the cache, then read one Key Vault secret into a shell variable with a TTL and explain output, checking it is non-empty without printing it; explain should show it came via the daemon. Exit 75 means throttled (do not retry) and HTTP 401, 403 or 404 errors are reported as printed.
