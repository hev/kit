---
id: "secrets#3-read-a-1password-secret"
title: "Secrets"
heading: "3. Read a 1Password secret"
group: "Operate"
order: 367
url: "/docs/secrets#3-read-a-1password-secret"
anchor: "3-read-a-1password-secret"
terms: ["read","1password","secret","short","example","explicit","reference","shell","variable","label","explain","password","secrets","vault","item","field","dbpassword"]
hash: "1090ac2eae060ef0bd996307797bd0c78233d3923f870380ecebe1f9792f8b78"
mode: "agent-primary"
facts: [{"kind":"code","literal":"DB_PASSWORD=$(hev secrets read --label DB_PASSWORD --ttl 1h --explain op://VAULT/ITEM/FIELD)","chunkId":"secrets#3-read-a-1password-secret"}]
sources: [{"chunkId":"secrets#3-read-a-1password-secret","url":"/docs/secrets#3-read-a-1password-secret","anchor":"3-read-a-1password-secret"}]
---

Short example for 1Password: read a secret by its explicit op reference into a shell variable with label, TTL and explain.
