---
id: "secrets#3-verify-key-vault"
title: "Secrets"
heading: "3. Verify Key Vault"
group: "Operate"
order: 254
url: "/docs/secrets#3-verify-key-vault"
anchor: "3-verify-key-vault"
terms: ["verify","vault","third","step","setup","explain","output","should","show","read","went","daemon","direct","means","caller","hold","same","azure","credential","status","command","shows","type","last","reachability","hits","misses","requests","cooldown","secrets","quota","says","doesn","compare","their","environments","whether","reachable","real","request"]
hash: "5a42b4ad78534546e6114632b804da970998bbdcb909333d7b0d3894d0625a5f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--explain","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"via=daemon","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"via=direct","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"hev secrets status --no-quota","chunkId":"secrets#3-verify-key-vault"}]
sources: [{"chunkId":"secrets#3-verify-key-vault","url":"/docs/secrets#3-verify-key-vault","anchor":"3-verify-key-vault"}]
---

Third step of Key Vault setup: the explain output should show the read went via the daemon; a direct read means the daemon and caller do not hold the same Azure credential. The status command shows a Key Vault row with credential type, last reachability, hits, misses, requests and any cooldown.
