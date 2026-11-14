---
id: "secrets#1-put-the-value-in-the-vault"
title: "Secrets"
heading: "1. Put the value in the vault"
group: "Operate"
order: 291
url: "/docs/secrets#1-put-the-value-in-the-vault"
anchor: "1-put-the-value-in-the-vault"
terms: ["value","vault","first","step","moving","secret","disk","store","tools","since","only","reads","secrets","1password","field","copy","reference","private","temp","file","never","appears","command","line","noting","names","allow","letters","digits","hyphens","azure","write","temporary","remove","umask","grep","stripe","keyvault","name","output"]
hash: "bd9bed9aa0dd6fa67f36033c077256cbb85ffa3afd321e43049b508d53d2c333"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# Azure Key Vault: write the value to a private temporary file, set it, remove the file\numask 077\ngrep '^STRIPE_API_KEY=' .env | cut -d= -f2- | tr -d '\\n' \u003e /tmp/stripe-api-key\naz keyvault secret set --vault-name VAULT --name stripe-api-key --file /tmp/stripe-api-key --output none\nrm /tmp/stripe-api-key","chunkId":"secrets#1-put-the-value-in-the-vault"},{"kind":"code","literal":"op://VAULT/ITEM/FIELD","chunkId":"secrets#1-put-the-value-in-the-vault"},{"kind":"code","literal":"STRIPE_API_KEY","chunkId":"secrets#1-put-the-value-in-the-vault"},{"kind":"code","literal":"stripe-api-key","chunkId":"secrets#1-put-the-value-in-the-vault"},{"kind":"code","literal":"azkv://VAULT/stripe-api-key","chunkId":"secrets#1-put-the-value-in-the-vault"}]
sources: [{"chunkId":"secrets#1-put-the-value-in-the-vault","url":"/docs/secrets#1-put-the-value-in-the-vault","anchor":"1-put-the-value-in-the-vault"}]
---

First step of moving a secret off disk: store the value with the vault's own tools, since kit only reads secrets. In 1Password, add a field and copy its secret reference; in Key Vault, set the secret from a private temp file so the value never appears on a command line, noting Key Vault names allow only letters, digits and hyphens.
