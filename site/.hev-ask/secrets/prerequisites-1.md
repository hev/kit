---
id: "secrets#prerequisites-1"
title: "Secrets"
heading: "Prerequisites"
group: "Operate"
order: 255
url: "/docs/secrets#prerequisites-1"
anchor: "prerequisites-1"
terms: ["prerequisites","azure","vault","setup","needs","secret","names","identity","secrets","reader","role","credential","hevd","running","upgraded","before","callers","references","neither","required","unless","choose","azkv","need","credentials","host","install","caller","uses","needed","hevazkvcredential"]
hash: "351dd58a557e9efe3b186d252e141eb9d06c58ae02d1fafb014eed33ad0af35d"
mode: "agent-primary"
facts: [{"kind":"code","literal":"azkv://","chunkId":"secrets#prerequisites-1"},{"kind":"code","literal":"op","chunkId":"secrets#prerequisites-1"},{"kind":"code","literal":"HEV_AZKV_CREDENTIAL=azure-cli","chunkId":"secrets#prerequisites-1"}]
sources: [{"chunkId":"secrets#prerequisites-1","url":"/docs/secrets#prerequisites-1","anchor":"prerequisites-1"}]
---

Azure Key Vault setup needs a vault, the secret names, an identity with the Key Vault secrets reader role, one credential for that identity, and hev with hevd running, upgraded before callers use Key Vault references. Neither op nor the Azure CLI is required unless you choose the CLI credential.
