---
id: "secrets#a-windows-host-with-only-key-vault"
title: "Secrets"
heading: "A Windows host with only Key Vault"
group: "Operate"
order: 307
url: "/docs/secrets#a-windows-host-with-only-key-vault"
anchor: "a-windows-host-with-only-key-vault"
terms: ["windows","host","only","vault","alone","running","daemon","secrets","mode","managed","identity","azure","variables","server","tenant","client","certificate","settings","elsewhere","needs","1password","token","file","callers","same","commands","references","password","read","label","azkv","secret","program","credential","path","service","account","hevd","hevazkvcredential","connected"]
hash: "b5e500aebd182600db089e0e89d54dbdc21c007e39aeb4d22ac3a350ca15aaa4"
mode: "agent-primary"
facts: [{"kind":"code","literal":"$env:DB_PASSWORD = hev secrets read --label DB_PASSWORD --ttl 1h azkv://VAULT/SECRET\nhev secrets run --ref DB_PASSWORD=azkv://VAULT/SECRET --ttl DB_PASSWORD=1h -- program.exe","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"hev daemon --secrets-only","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"HEV_AZKV_CREDENTIAL=managed-identity","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_TENANT_ID","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_CLIENT_ID","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_CLIENT_CERTIFICATE_PATH","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"op","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"OP_SERVICE_ACCOUNT_TOKEN","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"azkv://","chunkId":"secrets#a-windows-host-with-only-key-vault"}]
sources: [{"chunkId":"secrets#a-windows-host-with-only-key-vault","url":"/docs/secrets#a-windows-host-with-only-key-vault","anchor":"a-windows-host-with-only-key-vault"}]
---

A Windows host can use Key Vault alone by running the daemon, or its secrets-only mode, with a managed identity on an Azure VM, Arc's own variables on an Arc server, or tenant, client and certificate settings elsewhere. It needs no 1Password CLI, token or token file, and callers use the same commands with Key Vault references.
