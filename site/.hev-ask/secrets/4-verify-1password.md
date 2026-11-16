---
id: "secrets#4-verify-1password"
title: "Secrets"
heading: "4. Verify 1Password"
group: "Operate"
order: 306
url: "/docs/secrets#4-verify-1password"
anchor: "4-verify-1password"
terms: ["verify","1password","explanation","line","shows","whether","value","came","real","read","cache","daemon","served","repeating","should","status","naming","token","file","environment","explain","secrets","password","source","direct","quota","service","account","prints","reference","stderr","such","dbpassword","means","call","made","request","memory","same","again"]
hash: "6075aa454f5532610b253ff173df2508b1e42a33ad80c0c304f5402786768388"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--explain","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"hev secrets: DB_PASSWORD source=read via=daemon","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"source=read","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"source=cache","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"via=direct","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"hev secrets status --no-quota","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"service-account token (token file)","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"service-account token (environment)","chunkId":"secrets#4-verify-1password"}]
sources: [{"chunkId":"secrets#4-verify-1password","url":"/docs/secrets#4-verify-1password","anchor":"4-verify-1password"}]
---

To verify 1Password, the explanation line shows whether a value came from a real read or the cache and whether the daemon served it; repeating the read should hit the cache. Status shows a 1Password row naming whether the token came from the token file or the environment.
