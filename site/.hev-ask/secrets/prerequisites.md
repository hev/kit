---
id: "secrets#prerequisites"
title: "Secrets"
heading: "Prerequisites"
group: "Operate"
order: 254
url: "/docs/secrets#prerequisites"
anchor: "prerequisites"
terms: ["prerequisites","1password","setup","needs","service","account","read","access","needed","vaults","token","desktop","biometric","unlock","never","path","daemon","every","caller","hevd","running","secrets","only","endpoint","need","uses","host","install","switch","cache"]
hash: "bca1f564e1851be8b1bd508d506258ae680b4983f2675f7c201a2da41dd21121"
mode: "agent-primary"
facts: [{"kind":"code","literal":"op","chunkId":"secrets#prerequisites"}]
sources: [{"chunkId":"secrets#prerequisites","url":"/docs/secrets#prerequisites","anchor":"prerequisites"}]
---

1Password setup needs a service account with read access to the needed vaults and its token (the desktop app and biometric unlock are never used), the op CLI on the PATH of the daemon and every caller, and hev with hevd running or the secrets-only endpoint.
