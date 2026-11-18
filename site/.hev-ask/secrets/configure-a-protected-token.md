---
id: "secrets#configure-a-protected-token"
title: "Secrets"
heading: "Configure a protected token"
group: "Operate"
order: 310
url: "/docs/secrets#configure-a-protected-token"
anchor: "configure-a-protected-token"
terms: ["configure","protected","token","windows","1password","service","account","live","dpapi","file","both","daemon","callers","machine","user","scope","optional","fixed","entropy","appears","task","arguments","environment","every","read","checks","strict","acls","ownership","directory","unusable","store","fails","closed","rather","falling","back","plaintext","there","commands"]
hash: "4863660675efa9fcd8692c542a5cd8c8f76dbe4e42c14edf190930db7e1dc680"
mode: "agent-primary"
facts: [{"kind":"code","literal":"$env:HEV_OP_DPAPI_FILE = 'C:\\Example\\CredentialStore\\token.dpapi'\n$env:HEV_OP_DPAPI_SCOPE = 'LocalMachine'\n$env:HEV_OP_DPAPI_ENTROPY = 'example-fixed-entropy'\nRemove-Item Env:OP_SERVICE_ACCOUNT_TOKEN -ErrorAction SilentlyContinue\nhev secrets token-store inspect","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"HEV_OP_DPAPI_FILE","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":".NET ProtectedData.Protect","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"LocalMachine","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"CurrentUser","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"HEV_OP_DPAPI_ENTROPY","chunkId":"secrets#configure-a-protected-token"},{"kind":"code","literal":"HEV_OP_TOKEN_FILE","chunkId":"secrets#configure-a-protected-token"},{"kind":"value","literal":"UTF-8","chunkId":"secrets#configure-a-protected-token"}]
sources: [{"chunkId":"secrets#configure-a-protected-token","url":"/docs/secrets#configure-a-protected-token","anchor":"configure-a-protected-token"}]
---

On Windows the 1Password service-account token can live in a DPAPI-protected file used by both the daemon and callers, in machine or user scope with optional fixed entropy, so no token appears in task arguments or environment. Every read checks strict ACLs and ownership on the file and its directory, an unusable store fails closed rather than falling back to a plaintext file, and there are commands to create a new store from standard input and to inspect one without decrypting.
