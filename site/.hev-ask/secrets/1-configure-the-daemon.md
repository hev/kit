---
id: "secrets#1-configure-the-daemon"
title: "Secrets"
heading: "1. Configure the daemon"
group: "Operate"
order: 245
url: "/docs/secrets#1-configure-the-daemon"
anchor: "1-configure-the-daemon"
terms: ["configure","daemon","first","step","vault","setup","azure","credential","settings","environment","example","systemd","user","drop","pointing","client","certificate","linux","managed","identity","only","selection","needed","plus","assigned","windows","covered","separately","config","hevd","service","azkv","conf","tenant","path","variables","azuretenantid","tenantid","azureclientid","clientid"]
hash: "e24108e4fcd23a4083cd4efae1d51d2ac3c2799052b9f97faaff86ea3f683a79"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# ~/.config/systemd/user/hevd.service.d/azkv.conf\n[Service]\nEnvironment=AZURE_TENANT_ID=TENANT_ID\nEnvironment=AZURE_CLIENT_ID=CLIENT_ID\nEnvironment=AZURE_CLIENT_CERTIFICATE_PATH=%h/.config/azkv/client.pem","chunkId":"secrets#1-configure-the-daemon"},{"kind":"code","literal":"Environment=HEV_AZKV_CREDENTIAL=managed-identity","chunkId":"secrets#1-configure-the-daemon"},{"kind":"code","literal":"AZURE_CLIENT_ID","chunkId":"secrets#1-configure-the-daemon"}]
sources: [{"chunkId":"secrets#1-configure-the-daemon","url":"/docs/secrets#1-configure-the-daemon","anchor":"1-configure-the-daemon"}]
---

First step of Key Vault setup: put the Azure credential's settings in the daemon's environment, for example as a systemd user drop-in pointing at a client certificate on Linux. On an Azure VM with a managed identity only the managed-identity selection is needed, plus the client ID for a user-assigned identity; Windows is covered separately.
