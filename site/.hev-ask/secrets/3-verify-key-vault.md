---
id: "secrets#3-verify-key-vault"
title: "Secrets"
heading: "3. Verify Key Vault"
group: "Operate"
order: 368
url: "/docs/secrets#3-verify-key-vault"
anchor: "3-verify-key-vault"
terms: ["verify","vault","setup","explain","output","should","report","read","came","daemon","direct","means","lacks","same","azure","credential","caller","status","command","shows","type","reachability","secrets","quota","says","doesn","hold","compare","their","environments","whether","reachable","last","real","request","hits","misses","requests","cooldown"]
hash: "5a42b4ad78534546e6114632b804da970998bbdcb909333d7b0d3894d0625a5f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--explain","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"via=daemon","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"via=direct","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"hev secrets status --no-quota","chunkId":"secrets#3-verify-key-vault"}]
sources: [{"chunkId":"secrets#3-verify-key-vault","url":"/docs/secrets#3-verify-key-vault","anchor":"3-verify-key-vault"}]
---

Verify Key Vault setup: explain output should report the read came via the daemon; a direct read means the daemon lacks the same Azure credential as the caller. The status command shows a Key Vault row with credential type and reachability.
