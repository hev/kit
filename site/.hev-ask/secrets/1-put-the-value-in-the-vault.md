---
id: "secrets#1-put-the-value-in-the-vault"
title: "Secrets"
heading: "1. Put the value in the vault"
group: "Operate"
order: 247
url: "/docs/secrets#1-put-the-value-in-the-vault"
anchor: "1-put-the-value-in-the-vault"
terms: ["value","vault","first","step","replacing","secret","disk","store","tools","since","only","reads","secrets","1password","field","copy","reference","azure","private","temporary","file","never","appears","command","line","hyphenated","name","because","names","allow","letters","digits","hyphens","write","remove","umask","grep","stripe","keyvault","output"]
hash: "b7e67ab195f8435d3fe63ffe2020b53c1516fee9bd46b299e132348bc4d04842"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# Azure Key Vault: write the value to a private temporary file, set it, remove the file\numask 077\ngrep '^STRIPE_API_KEY=' .env | cut -d= -f2- | tr -d '\\n' \u003e /tmp/stripe-api-key\naz keyvault secret set --vault-name VAULT --name stripe-api-key --file /tmp/stripe-api-key --output none\nrm /tmp/stripe-api-key","chunkId":"secrets#1-put-the-value-in-the-vault"},{"kind":"code","literal":"op://VAULT/ITEM/FIELD","chunkId":"secrets#1-put-the-value-in-the-vault"},{"kind":"code","literal":"STRIPE_API_KEY","chunkId":"secrets#1-put-the-value-in-the-vault"},{"kind":"code","literal":"stripe-api-key","chunkId":"secrets#1-put-the-value-in-the-vault"},{"kind":"code","literal":"azkv://VAULT/stripe-api-key","chunkId":"secrets#1-put-the-value-in-the-vault"}]
sources: [{"chunkId":"secrets#1-put-the-value-in-the-vault","url":"/docs/secrets#1-put-the-value-in-the-vault","anchor":"1-put-the-value-in-the-vault"}]
---

First step of replacing a secret on disk: store the value using the vault's own tools, since kit only reads secrets. In 1Password, add it as a field and copy its secret reference; in Azure Key Vault, set it from a private temporary file so it never appears on a command line, using a hyphenated name because Key Vault names allow only letters, digits and hyphens.
