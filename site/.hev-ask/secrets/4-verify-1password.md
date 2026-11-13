---
id: "secrets#4-verify-1password"
title: "Secrets"
heading: "4. Verify 1Password"
group: "Operate"
order: 256
url: "/docs/secrets#4-verify-1password"
anchor: "4-verify-1password"
terms: ["verify","1password","final","step","setup","explain","line","stderr","shows","whether","value","came","real","request","cache","daemon","served","read","directly","repeated","should","status","command","naming","token","file","environment","secrets","password","source","direct","quota","service","account","prints","reference","such","dbpassword","means","call"]
hash: "6075aa454f5532610b253ff173df2508b1e42a33ad80c0c304f5402786768388"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--explain","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"hev secrets: DB_PASSWORD source=read via=daemon","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"source=read","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"source=cache","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"via=direct","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"hev secrets status --no-quota","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"service-account token (token file)","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"service-account token (environment)","chunkId":"secrets#4-verify-1password"}]
sources: [{"chunkId":"secrets#4-verify-1password","url":"/docs/secrets#4-verify-1password","anchor":"4-verify-1password"}]
---

Final step of 1Password setup: the explain line on stderr shows whether a value came from a real request or from the cache, and whether a daemon served it or it was read directly. A repeated read should be served from cache, and the status command shows a 1Password row naming whether the token came from the token file or the environment.
