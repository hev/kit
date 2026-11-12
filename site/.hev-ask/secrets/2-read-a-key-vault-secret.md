---
id: "secrets#2-read-a-key-vault-secret"
title: "Secrets"
heading: "2. Read a Key Vault secret"
group: "Operate"
order: 242
url: "/docs/secrets#2-read-a-key-vault-secret"
anchor: "2-read-a-key-vault-secret"
terms: ["read","vault","secret","after","checking","cache","shell","variable","label","explain","output","never","printing","value","line","should","came","daemon","exit","means","throttled","request","retried","http","errors","reported","printed","secrets","ping","password","azkv","dbpassword","azure","through","first","print","names","give","echo","only"]
hash: "6e261065788dd91f01a5496d3514155d9082e333de00eb7e7ea233585999b9af"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets ping\nDB_PASSWORD=$(hev secrets read --label DB_PASSWORD --ttl 1h --explain azkv://VAULT/SECRET)","chunkId":"secrets#2-read-a-key-vault-secret"}]
sources: [{"chunkId":"secrets#2-read-a-key-vault-secret","url":"/docs/secrets#2-read-a-key-vault-secret","anchor":"2-read-a-key-vault-secret"}]
---

After checking the cache is up, read a Key Vault secret into a shell variable with a label, a TTL and explain output, never printing the value. The explain line should say it came via the daemon; exit 75 means the vault throttled the request and should not be retried, and HTTP 401, 403 or 404 errors should be reported as printed.
