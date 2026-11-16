---
id: "secrets#a-windows-host-with-only-key-vault"
title: "Secrets"
heading: "A Windows host with only Key Vault"
group: "Operate"
order: 257
url: "/docs/secrets#a-windows-host-with-only-key-vault"
anchor: "a-windows-host-with-only-key-vault"
terms: ["windows","host","only","vault","uses","azure","hevd","secrets","daemon","managed","identity","nothing","extra","connected","server","service","principal","certificate","elsewhere","needs","1password","token","callers","same","read","commands","references","powershell","password","label","azkv","secret","program","credential","tenant","client","path","account","hevazkvcredential","beyond"]
hash: "b5e500aebd182600db089e0e89d54dbdc21c007e39aeb4d22ac3a350ca15aaa4"
mode: "agent-primary"
facts: [{"kind":"code","literal":"$env:DB_PASSWORD = hev secrets read --label DB_PASSWORD --ttl 1h azkv://VAULT/SECRET\nhev secrets run --ref DB_PASSWORD=azkv://VAULT/SECRET --ttl DB_PASSWORD=1h -- program.exe","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"hev daemon --secrets-only","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"HEV_AZKV_CREDENTIAL=managed-identity","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_TENANT_ID","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_CLIENT_ID","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_CLIENT_CERTIFICATE_PATH","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"op","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"OP_SERVICE_ACCOUNT_TOKEN","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"azkv://","chunkId":"secrets#a-windows-host-with-only-key-vault"}]
sources: [{"chunkId":"secrets#a-windows-host-with-only-key-vault","url":"/docs/secrets#a-windows-host-with-only-key-vault","anchor":"a-windows-host-with-only-key-vault"}]
---

A Windows host that uses only Azure Key Vault can run hevd or the secrets-only daemon with a managed identity on an Azure VM, nothing extra on an Arc-connected server, or a service principal certificate elsewhere. It needs no 1Password CLI or token, and callers use the same read and run commands with Key Vault references in PowerShell.
