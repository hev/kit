---
id: "secrets#4-verify-1password"
title: "Secrets"
heading: "4. Verify 1Password"
group: "Operate"
order: 300
url: "/docs/secrets#4-verify-1password"
anchor: "4-verify-1password"
terms: ["verify","1password","explain","output","prints","line","reference","showing","whether","real","read","cache","went","daemon","directly","repeat","should","status","shows","token","source","secrets","password","direct","quota","service","account","file","environment","stderr","such","dbpassword","means","call","made","request","served","memory","same","again"]
hash: "6075aa454f5532610b253ff173df2508b1e42a33ad80c0c304f5402786768388"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--explain","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"hev secrets: DB_PASSWORD source=read via=daemon","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"source=read","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"source=cache","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"via=direct","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"hev secrets status --no-quota","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"service-account token (token file)","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"service-account token (environment)","chunkId":"secrets#4-verify-1password"}]
sources: [{"chunkId":"secrets#4-verify-1password","url":"/docs/secrets#4-verify-1password","anchor":"4-verify-1password"}]
---

Explain output prints one line per reference showing whether it was a real read or a cache hit and whether it went via the daemon or directly; a repeat read should be a cache hit. Status shows the 1Password row with the token source.
