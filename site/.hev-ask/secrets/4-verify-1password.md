---
id: "secrets#4-verify-1password"
title: "Secrets"
heading: "4. Verify 1Password"
group: "Operate"
order: 246
url: "/docs/secrets#4-verify-1password"
anchor: "4-verify-1password"
terms: ["verify","1password","explain","prints","line","reference","showing","whether","value","came","real","request","memory","cache","daemon","served","repeated","read","should","come","status","shows","service","account","token","credential","file","environment","secrets","password","source","direct","quota","stderr","such","dbpassword","means","call","made","same"]
hash: "6075aa454f5532610b253ff173df2508b1e42a33ad80c0c304f5402786768388"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--explain","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"hev secrets: DB_PASSWORD source=read via=daemon","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"source=read","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"source=cache","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"via=direct","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"hev secrets status --no-quota","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"service-account token (token file)","chunkId":"secrets#4-verify-1password"},{"kind":"code","literal":"service-account token (environment)","chunkId":"secrets#4-verify-1password"}]
sources: [{"chunkId":"secrets#4-verify-1password","url":"/docs/secrets#4-verify-1password","anchor":"4-verify-1password"}]
---

Explain prints one line per reference showing whether the value came from a real request or the in-memory cache and whether a daemon served it; a repeated read should come from cache. Status shows the 1Password row with the service-account token credential and whether it came from the token file or the environment.
