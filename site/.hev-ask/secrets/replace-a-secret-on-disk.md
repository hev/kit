---
id: "secrets#replace-a-secret-on-disk"
title: "Secrets"
heading: "Replace a secret on disk"
group: "Operate"
order: 266
url: "/docs/secrets#replace-a-secret-on-disk"
anchor: "replace-a-secret-on-disk"
terms: ["replace","secret","disk","walkthrough","moving","file","start","script","loads","afterwards","vault","holds","value","only","reference","gone","store","first","follow","numbered","steps","order","before","stripe","live","example","exec","server","1password","moves","project","holding","real","stripeapikey","skliveexamplevalue","after","azure"]
hash: "66b024c310447b652db43a1fd571d7ae9538e2e31062cb3e1c74297f51824118"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# .env (before): the value is on disk\nSTRIPE_API_KEY=sk_live_EXAMPLE_VALUE","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"# run.sh (before)\nset -a; . ./.env; set +a\nexec ./server","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":".env","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"value","literal":"set-up-1password","chunkId":"secrets#replace-a-secret-on-disk"}]
sources: [{"chunkId":"secrets#replace-a-secret-on-disk","url":"/docs/secrets#replace-a-secret-on-disk","anchor":"replace-a-secret-on-disk"}]
---

Walkthrough for moving one secret out of a .env file that a start script loads: afterwards the vault holds the value, the script holds only a reference, and the .env file is gone. Set up the store first, then follow the numbered steps in order.
