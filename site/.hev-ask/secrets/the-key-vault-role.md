---
id: "secrets#the-key-vault-role"
title: "Secrets"
heading: "The Key Vault role"
group: "Operate"
order: 271
url: "/docs/secrets#the-key-vault-role"
anchor: "the-key-vault-role"
terms: ["vault","role","azure","identity","uses","needs","only","secrets","assign","built","user","scoped","access","policy","grant","secret","permission","never","lists","sets","deletes","signs","nothing","more","give","rbac","still","policies"]
hash: "39d535a395088430526f5060b022b7890ab6c11ea185d780a6e6c1e67a504ac5"
mode: "agent-primary"
facts: []
sources: [{"chunkId":"secrets#the-key-vault-role","url":"/docs/secrets#the-key-vault-role","anchor":"the-key-vault-role"}]
---

The Azure identity kit uses needs only to get secrets: assign the built-in Key Vault Secrets User role scoped to the vault, or on an access-policy vault grant only the Get secret permission. kit never lists, sets or deletes secrets.
