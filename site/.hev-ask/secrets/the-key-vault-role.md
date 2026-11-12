---
id: "secrets#the-key-vault-role"
title: "Secrets"
heading: "The Key Vault role"
group: "Operate"
order: 260
url: "/docs/secrets#the-key-vault-role"
anchor: "the-key-vault-role"
terms: ["vault","role","azure","identity","signs","only","needs","secrets","grant","built","user","scoped","secret","permission","under","access","policies","never","lists","sets","deletes","nothing","more","give","rbac","still","uses"]
hash: "39d535a395088430526f5060b022b7890ab6c11ea185d780a6e6c1e67a504ac5"
mode: "agent-primary"
facts: []
sources: [{"chunkId":"secrets#the-key-vault-role","url":"/docs/secrets#the-key-vault-role","anchor":"the-key-vault-role"}]
---

The Azure identity kit signs in as only needs to get secrets: grant the built-in Key Vault Secrets User role scoped to the vault, or only the Get secret permission under access policies. kit never lists, sets or deletes secrets.
