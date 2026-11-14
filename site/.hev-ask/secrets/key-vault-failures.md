---
id: "secrets#key-vault-failures"
title: "Secrets"
heading: "Key Vault failures"
group: "Operate"
order: 306
url: "/docs/secrets#key-vault-failures"
anchor: "key-vault-failures"
terms: ["vault","failures","troubleshooting","table","errors","credential","configured","missing","secret","read","role","tenant","client","version","sign","unsupported","certificate","formats","encrypted","keys","azure","partial","platform","identity","environment","unreadable","file","reference","cycles","unknown","clouds","exit","throttling","denied","access","http","principal","needs","secrets","refused"]
hash: "973e177b8068201fb58e7bb8e9191584cfa6ecef06a53bd978b3152de8878b69"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Azure Key Vault has no credential configured; ...","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault denied access (HTTP 403): the ... principal needs get on secrets for this vault","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault refused the ... credential (HTTP 401)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault has no such secret or version (HTTP 404)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure sign-in failed for the ... credential (HTTP ...)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure client certificate: the PEM file needs a certificate and an unencrypted RSA private key","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure CLI credential selected but az is not on PATH","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault managed identity: IDENTITY_ENDPOINT needs IDENTITY_HEADER (App Service) or IMDS_ENDPOINT (Azure Arc)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"HEV_AZKV_CREDENTIAL","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Arc identity key file is unreadable (the user must be in the agent's token group)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault: AZURE_CLIENT_SECRET_REF cannot be an azkv:// reference (credential reference cycle); use another provider","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"set AZURE_AUTHORITY_HOST for a vault outside Azure's known clouds","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"AZURE_AUTHORITY_HOST","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"HEV_AZKV_DNS_SUFFIXES","chunkId":"secrets#key-vault-failures"}]
sources: [{"chunkId":"secrets#key-vault-failures","url":"/docs/secrets#key-vault-failures","anchor":"key-vault-failures"}]
---

Troubleshooting table for Key Vault errors: no credential configured, 403 missing secret-read role, 401 bad tenant or client, 404 missing secret or version, sign-in failures, unsupported certificate formats (PFX or encrypted keys), missing Azure CLI, partial platform identity environment, unreadable Arc key file, credential reference cycles, unknown clouds, and exit 75 throttling.
