---
id: "secrets#key-vault-failures"
title: "Secrets"
heading: "Key Vault failures"
group: "Operate"
order: 252
url: "/docs/secrets#key-vault-failures"
anchor: "key-vault-failures"
terms: ["vault","failures","errors","fixes","credential","configured","message","names","variables","http","needing","secrets","role","wrong","tenant","client","missing","secret","version","sign","files","password","protected","azure","partial","managed","identity","environments","token","group","permissions","reference","cycle","back","stack","authority","suffix","settings","exit","throttling"]
hash: "973e177b8068201fb58e7bb8e9191584cfa6ecef06a53bd978b3152de8878b69"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Azure Key Vault has no credential configured; ...","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault denied access (HTTP 403): the ... principal needs get on secrets for this vault","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault refused the ... credential (HTTP 401)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault has no such secret or version (HTTP 404)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure sign-in failed for the ... credential (HTTP ...)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure client certificate: the PEM file needs a certificate and an unencrypted RSA private key","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure CLI credential selected but az is not on PATH","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault managed identity: IDENTITY_ENDPOINT needs IDENTITY_HEADER (App Service) or IMDS_ENDPOINT (Azure Arc)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"HEV_AZKV_CREDENTIAL","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Arc identity key file is unreadable (the user must be in the agent's token group)","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"Azure Key Vault: AZURE_CLIENT_SECRET_REF cannot be an azkv:// reference (credential reference cycle); use another provider","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"set AZURE_AUTHORITY_HOST for a vault outside Azure's known clouds","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"AZURE_AUTHORITY_HOST","chunkId":"secrets#key-vault-failures"},{"kind":"code","literal":"HEV_AZKV_DNS_SUFFIXES","chunkId":"secrets#key-vault-failures"}]
sources: [{"chunkId":"secrets#key-vault-failures","url":"/docs/secrets#key-vault-failures","anchor":"key-vault-failures"}]
---

Key Vault errors and fixes: no credential configured (the message names variables to set), HTTP 403 needing the secrets get role, HTTP 401 from wrong tenant or client IDs, HTTP 404 for a missing secret or version, sign-in failures, PEM files that are PFX or password-protected, a missing Azure CLI, partial managed identity environments, Arc token group permissions, a client secret reference cycle back to Key Vault, Azure Stack Hub needing authority and DNS suffix settings, and exit 75 for throttling.
