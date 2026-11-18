---
id: "secrets#3-verify-key-vault"
title: "Secrets"
heading: "3. Verify Key Vault"
group: "Operate"
order: 305
url: "/docs/secrets#3-verify-key-vault"
anchor: "3-verify-key-vault"
terms: ["verify","vault","read","explanation","should","show","served","daemon","direct","means","lacks","caller","azure","credential","free","status","shows","type","reachability","hits","misses","requests","cooldown","explain","secrets","quota","says","doesn","hold","same","compare","their","environments","whether","reachable","last","real","request"]
hash: "5a42b4ad78534546e6114632b804da970998bbdcb909333d7b0d3894d0625a5f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--explain","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"via=daemon","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"via=direct","chunkId":"secrets#3-verify-key-vault"},{"kind":"code","literal":"hev secrets status --no-quota","chunkId":"secrets#3-verify-key-vault"}]
sources: [{"chunkId":"secrets#3-verify-key-vault","url":"/docs/secrets#3-verify-key-vault","anchor":"3-verify-key-vault"}]
---

To verify Key Vault, the read's explanation should show it was served via the daemon; a direct read means the daemon lacks the caller's Azure credential. Free status shows a Key Vault row with credential type, reachability, hits, misses, requests and any cooldown.
