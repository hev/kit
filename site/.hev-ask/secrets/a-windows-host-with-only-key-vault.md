---
id: "secrets#a-windows-host-with-only-key-vault"
title: "Secrets"
heading: "A Windows host with only Key Vault"
group: "Operate"
order: 301
url: "/docs/secrets#a-windows-host-with-only-key-vault"
anchor: "a-windows-host-with-only-key-vault"
terms: ["windows","host","only","vault","azure","runs","daemon","secrets","managed","identity","variables","server","service","principal","certificate","elsewhere","needing","1password","tooling","callers","read","powershell","pass","program","same","unix","password","label","azkv","secret","credential","tenant","client","path","account","token","hevd","hevazkvcredential","connected","nothing"]
hash: "3325b24cbf020bf8746db5b51ff51945de5121144e4642ca0b7e7634dd0e3380"
mode: "agent-primary"
facts: [{"kind":"code","literal":"$env:DB_PASSWORD = hev secrets read --label DB_PASSWORD --ttl 1h azkv://VAULT/SECRET\nhev secrets run --ref DB_PASSWORD=azkv://VAULT/SECRET --ttl DB_PASSWORD=1h -- program.exe","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"hev daemon --secrets-only","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"HEV_AZKV_CREDENTIAL=managed-identity","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_TENANT_ID","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_CLIENT_ID","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_CLIENT_CERTIFICATE_PATH","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"op","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"OP_SERVICE_ACCOUNT_TOKEN","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"azkv://","chunkId":"secrets#a-windows-host-with-only-key-vault"}]
sources: [{"chunkId":"secrets#a-windows-host-with-only-key-vault","url":"/docs/secrets#a-windows-host-with-only-key-vault","anchor":"a-windows-host-with-only-key-vault"}]
---

A Windows host using only Azure Key Vault runs the daemon (or a secrets-only daemon) with a managed identity on an Azure VM, Arc's own variables on an Arc server, or service principal certificate variables elsewhere, needing no 1Password tooling. Callers read secrets into PowerShell variables or pass them to a program the same way as on Unix.
