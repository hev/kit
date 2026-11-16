---
id: "secrets#the-key-vault-role"
title: "Secrets"
heading: "The Key Vault role"
group: "Operate"
order: 328
url: "/docs/secrets#the-key-vault-role"
anchor: "the-key-vault-role"
terms: ["vault","role","azure","identity","uses","needs","only","permission","secrets","built","user","scoped","just","vaults","access","policies","never","lists","sets","deletes","signs","nothing","more","give","rbac","still","grant","secret"]
hash: "39d535a395088430526f5060b022b7890ab6c11ea185d780a6e6c1e67a504ac5"
mode: "agent-primary"
facts: []
sources: [{"chunkId":"secrets#the-key-vault-role","url":"/docs/secrets#the-key-vault-role","anchor":"the-key-vault-role"}]
---

The Azure identity kit uses needs only permission to get secrets: the built-in secrets-user role scoped to the vault, or just the Get permission on vaults using access policies. kit never lists, sets or deletes secrets.
