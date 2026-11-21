---
id: "secrets#2-read-a-key-vault-secret"
title: "Secrets"
heading: "2. Read a Key Vault secret"
group: "Operate"
order: 365
url: "/docs/secrets#2-read-a-key-vault-secret"
anchor: "2-read-a-key-vault-secret"
terms: ["read","vault","secret","short","example","ping","cache","explicit","reference","shell","variable","label","explain","output","secrets","password","azkv","dbpassword"]
hash: "68f5a1896709a5f6fca94b062b44960b8887aefef012640f536c63aa833d33e8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets ping\nDB_PASSWORD=$(hev secrets read --label DB_PASSWORD --ttl 1h --explain azkv://VAULT/SECRET)","chunkId":"secrets#2-read-a-key-vault-secret"}]
sources: [{"chunkId":"secrets#2-read-a-key-vault-secret","url":"/docs/secrets#2-read-a-key-vault-secret","anchor":"2-read-a-key-vault-secret"}]
---

Short example for Key Vault: ping the cache, then read a secret by its explicit Key Vault reference into a shell variable with a label, TTL and explain output.
