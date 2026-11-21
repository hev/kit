---
id: "secrets#key-vault-failures"
title: "Secrets"
heading: "Key Vault failures"
group: "Operate"
order: 377
url: "/docs/secrets#key-vault-failures"
anchor: "key-vault-failures"
terms: ["vault","failures","table","error","messages","causes","fixes","direct","read","fallback","daemon","lacks","same","azure","credential","plus","permission","throttling","problems","secrets","notice","azkv","configured","denied","access","http","principal","needs","refused","such","secret","version","sign","failed","client","certificate","file","unencrypted","private","selected"]
hash: "13be56785a7b1c6ee59816f947fac25e5e9f8db2a1b5c3f34197483f74cd0cce"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets: notice: fallback to direct Azure Key Vault read","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"AZURE_*","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"HEV_AZKV_CREDENTIAL","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault has no credential configured; ...","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault denied access (HTTP 403): the ... principal needs get on secrets for this vault","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault refused the ... credential (HTTP 401)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault has no such secret or version (HTTP 404)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure sign-in failed for the ... credential (HTTP ...)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure client certificate: the PEM file needs a certificate and an unencrypted RSA private key","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure CLI credential selected but az is not on PATH","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault managed identity: IDENTITY_ENDPOINT needs IDENTITY_HEADER (App Service) or IMDS_ENDPOINT (Azure Arc)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Arc identity key file is unreadable (the user must be in the agent's token group)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault: AZURE_CLIENT_SECRET_REF cannot be an azkv:// reference, nor a secret:// reference on a Key Vault host (credential reference cycle); use another provider","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"op://","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"set AZURE_AUTHORITY_HOST for a vault outside Azure's known clouds","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"AZURE_AUTHORITY_HOST","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"HEV_AZKV_DNS_SUFFIXES","chunkId":"secrets#key-vault-failures"}]
sources: [{"chunkId":"secrets#key-vault-failures","url":"/docs/secrets#key-vault-failures","anchor":"key-vault-failures"}]
---

Table of Key Vault error messages with causes and fixes, e.g. direct-read fallback when the daemon lacks the same Azure credential, plus permission and throttling problems.
