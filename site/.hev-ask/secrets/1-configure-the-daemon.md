---
id: "secrets#1-configure-the-daemon"
title: "Secrets"
heading: "1. Configure the daemon"
group: "Operate"
order: 289
url: "/docs/secrets#1-configure-the-daemon"
anchor: "1-configure-the-daemon"
terms: ["configure","daemon","azure","vault","putting","credential","variables","example","tenant","client","certificate","path","service","principal","environment","systemd","drop","exporting","same","callers","restarting","confirming","type","status","managed","identity","setting","suffices","windows","covered","separately","config","user","hevd","azkv","conf","linux","azuretenantid","tenantid","azureclientid"]
hash: "0470b2f327baaafeb0e0190356c2102152b7fd844a9e1e3a956cabe8ec211003"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# ~/.config/systemd/user/hevd.service.d/azkv.conf\n[Service]\nEnvironment=AZURE_TENANT_ID=TENANT_ID\nEnvironment=AZURE_CLIENT_ID=CLIENT_ID\nEnvironment=AZURE_CLIENT_CERTIFICATE_PATH=%h/.config/azkv/client.pem","chunkId":"secrets#1-configure-the-daemon"},{"kind":"code","literal":"Environment=HEV_AZKV_CREDENTIAL=managed-identity","chunkId":"secrets#1-configure-the-daemon"},{"kind":"code","literal":"AZURE_CLIENT_ID","chunkId":"secrets#1-configure-the-daemon"}]
sources: [{"chunkId":"secrets#1-configure-the-daemon","url":"/docs/secrets#1-configure-the-daemon","anchor":"1-configure-the-daemon"}]
---

Configure the daemon for Azure Key Vault by putting the credential variables (for example tenant, client and certificate path for a service principal) in its environment via a systemd drop-in, exporting the same in callers, restarting, and confirming the credential type in status. On an Azure VM a managed identity setting suffices; Windows is covered separately.
