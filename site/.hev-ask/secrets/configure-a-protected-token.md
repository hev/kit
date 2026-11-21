---
id: "secrets#configure-a-protected-token"
title: "Secrets"
heading: "Configure a protected token"
group: "Operate"
order: 373
url: "/docs/secrets#configure-a-protected-token"
anchor: "configure-a-protected-token"
terms: ["configure","protected","token","windows","dpapi","1password","store","through","environment","settings","file","machine","user","scope","optional","entropy","both","daemon","callers","task","arguments","inspect","command","example","credentialstore","localmachine","fixed","remove","item","service","account","erroraction","silentlycontinue","secrets","protecteddata","protect","currentuser","hevd","caller","these"]
hash: "4863660675efa9fcd8692c542a5cd8c8f76dbe4e42c14edf190930db7e1dc680"
mode: "agent-primary"
facts: [{"kind":"code","literal":"$env:HEV_OP_DPAPI_FILE = 'C:\\Example\\CredentialStore\\token.dpapi'\n$env:HEV_OP_DPAPI_SCOPE = 'LocalMachine'\n$env:HEV_OP_DPAPI_ENTROPY = 'example-fixed-entropy'\nRemove-Item Env:OP_SERVICE_ACCOUNT_TOKEN -ErrorAction SilentlyContinue\nhev secrets token-store inspect","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"HEV_OP_DPAPI_FILE","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":".NET ProtectedData.Protect","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"LocalMachine","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"CurrentUser","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"HEV_OP_DPAPI_ENTROPY","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"HEV_OP_TOKEN_FILE","chunkId":"secrets#configure-a-protected-token"},{"kind":"value","literal":"UTF-8","chunkId":"secrets#configure-a-protected-token"}]
sources: [{"chunkId":"secrets#configure-a-protected-token","url":"/docs/secrets#configure-a-protected-token","anchor":"configure-a-protected-token"}]
---

Windows: configure a DPAPI-protected 1Password token store through environment settings (file, machine or user scope, optional entropy) for both the daemon and callers, with no token in task arguments, and inspect it with the token-store command.
