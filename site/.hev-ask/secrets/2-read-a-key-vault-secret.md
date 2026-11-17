---
id: "secrets#2-read-a-key-vault-secret"
title: "Secrets"
heading: "2. Read a Key Vault secret"
group: "Operate"
order: 250
url: "/docs/secrets#2-read-a-key-vault-secret"
anchor: "2-read-a-key-vault-secret"
terms: ["read","vault","secret","second","step","setup","check","cache","shell","variable","label","hour","provenance","output","secrets","ping","password","explain","azkv","dbpassword"]
hash: "68f5a1896709a5f6fca94b062b44960b8887aefef012640f536c63aa833d33e8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets ping\nDB_PASSWORD=$(hev secrets read --label DB_PASSWORD --ttl 1h --explain azkv://VAULT/SECRET)","chunkId":"secrets#2-read-a-key-vault-secret"}]
sources: [{"chunkId":"secrets#2-read-a-key-vault-secret","url":"/docs/secrets#2-read-a-key-vault-secret","anchor":"2-read-a-key-vault-secret"}]
---

Second step of Key Vault setup: check the cache is up, then read a Key Vault secret into a shell variable with a label, an hour TTL and provenance output.
