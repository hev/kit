---
id: "secrets#1-configure-the-daemon"
title: "Secrets"
heading: "1. Configure the daemon"
group: "Operate"
order: 239
url: "/docs/secrets#1-configure-the-daemon"
anchor: "1-configure-the-daemon"
terms: ["configure","daemon","azure","vault","credential","environment","variables","hevd","linux","systemd","drop","tenant","client","certificate","path","export","same","calling","shells","because","only","serves","callers","holding","status","should","show","type","managed","identity","selector","user","assigned","needed","config","service","azkv","conf","azuretenantid","tenantid"]
hash: "0470b2f327baaafeb0e0190356c2102152b7fd844a9e1e3a956cabe8ec211003"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# ~/.config/systemd/user/hevd.service.d/azkv.conf\n[Service]\nEnvironment=AZURE_TENANT_ID=TENANT_ID\nEnvironment=AZURE_CLIENT_ID=CLIENT_ID\nEnvironment=AZURE_CLIENT_CERTIFICATE_PATH=%h/.config/azkv/client.pem","chunkId":"secrets#1-configure-the-daemon"},{"kind":"code","literal":"Environment=HEV_AZKV_CREDENTIAL=managed-identity","chunkId":"secrets#1-configure-the-daemon"},{"kind":"code","literal":"AZURE_CLIENT_ID","chunkId":"secrets#1-configure-the-daemon"}]
sources: [{"chunkId":"secrets#1-configure-the-daemon","url":"/docs/secrets#1-configure-the-daemon","anchor":"1-configure-the-daemon"}]
---

For Azure Key Vault, put the credential's environment variables into hevd's environment, on Linux as a systemd drop-in with tenant ID, client ID and a PEM certificate path, and export the same variables in calling shells because the daemon only serves callers holding the same credential. Status should show the certificate credential type; on an Azure VM with managed identity only a credential selector (and client ID for user-assigned identity) is needed.
