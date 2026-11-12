---
id: "secrets#a-windows-host-with-only-key-vault"
title: "Secrets"
heading: "A Windows host with only Key Vault"
group: "Operate"
order: 247
url: "/docs/secrets#a-windows-host-with-only-key-vault"
anchor: "a-windows-host-with-only-key-vault"
terms: ["windows","host","only","vault","uses","runs","hevd","secrets","daemon","managed","identity","azure","nothing","extra","connected","server","service","principal","certificate","elsewhere","needs","1password","token","callers","powershell","read","values","environment","variables","pass","program","same","unix","never","putting","arguments","stop","exit","password","label"]
hash: "3325b24cbf020bf8746db5b51ff51945de5121144e4642ca0b7e7634dd0e3380"
mode: "agent-primary"
facts: [{"kind":"code","literal":"$env:DB_PASSWORD = hev secrets read --label DB_PASSWORD --ttl 1h azkv://VAULT/SECRET\nhev secrets run --ref DB_PASSWORD=azkv://VAULT/SECRET --ttl DB_PASSWORD=1h -- program.exe","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"hev daemon --secrets-only","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"HEV_AZKV_CREDENTIAL=managed-identity","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_TENANT_ID","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_CLIENT_ID","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"AZURE_CLIENT_CERTIFICATE_PATH","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"op","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"OP_SERVICE_ACCOUNT_TOKEN","chunkId":"secrets#a-windows-host-with-only-key-vault"},{"kind":"code","literal":"azkv://","chunkId":"secrets#a-windows-host-with-only-key-vault"}]
sources: [{"chunkId":"secrets#a-windows-host-with-only-key-vault","url":"/docs/secrets#a-windows-host-with-only-key-vault","anchor":"a-windows-host-with-only-key-vault"}]
---

A Windows host that only uses Key Vault runs hevd or the secrets-only daemon with managed identity on an Azure VM, nothing extra on an Arc-connected server, or a service principal certificate elsewhere, and needs no op or 1Password token. Callers in PowerShell read values into environment variables or pass them to a program's environment the same way as on Unix, never putting values in arguments, and stop on exit 75.
