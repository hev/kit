---
id: "secrets#1-configure-the-daemon"
title: "Secrets"
heading: "1. Configure the daemon"
group: "Operate"
order: 299
url: "/docs/secrets#1-configure-the-daemon"
anchor: "1-configure-the-daemon"
terms: ["configure","daemon","azure","vault","setup","step","credential","variables","environment","example","systemd","drop","tenant","client","certificate","path","linux","just","managed","identity","setting","config","user","hevd","service","azkv","conf","azuretenantid","tenantid","azureclientid","clientid","azureclientcertificatepath","needs","only","hevazkvcredential","plus","assigned","windows","host"]
hash: "e24108e4fcd23a4083cd4efae1d51d2ac3c2799052b9f97faaff86ea3f683a79"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# ~/.config/systemd/user/hevd.service.d/azkv.conf\n[Service]\nEnvironment=AZURE_TENANT_ID=TENANT_ID\nEnvironment=AZURE_CLIENT_ID=CLIENT_ID\nEnvironment=AZURE_CLIENT_CERTIFICATE_PATH=%h/.config/azkv/client.pem","chunkId":"secrets#1-configure-the-daemon"},{"kind":"code","literal":"Environment=HEV_AZKV_CREDENTIAL=managed-identity","chunkId":"secrets#1-configure-the-daemon"},{"kind":"code","literal":"AZURE_CLIENT_ID","chunkId":"secrets#1-configure-the-daemon"}]
sources: [{"chunkId":"secrets#1-configure-the-daemon","url":"/docs/secrets#1-configure-the-daemon","anchor":"1-configure-the-daemon"}]
---

Azure Key Vault setup, step one: put the credential's variables in the daemon's environment, for example via a systemd drop-in with tenant, client and certificate path on Linux, or just the managed-identity setting on an Azure VM.
