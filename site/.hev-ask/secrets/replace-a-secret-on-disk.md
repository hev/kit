---
id: "secrets#replace-a-secret-on-disk"
title: "Secrets"
heading: "Replace a secret on disk"
group: "Operate"
order: 310
url: "/docs/secrets#replace-a-secret-on-disk"
anchor: "replace-a-secret-on-disk"
terms: ["replace","secret","disk","walkthrough","overview","moving","file","before","sits","start","script","loads","after","vault","holds","value","only","reference","deleted","store","first","follow","steps","order","stripe","live","example","exec","server","1password","moves","project","holding","real","stripeapikey","skliveexamplevalue","gone","azure"]
hash: "66b024c310447b652db43a1fd571d7ae9538e2e31062cb3e1c74297f51824118"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# .env (before): the value is on disk\nSTRIPE_API_KEY=sk_live_EXAMPLE_VALUE","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"# run.sh (before)\nset -a; . ./.env; set +a\nexec ./server","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":".env","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"value","literal":"set-up-1password","chunkId":"secrets#replace-a-secret-on-disk"}]
sources: [{"chunkId":"secrets#replace-a-secret-on-disk","url":"/docs/secrets#replace-a-secret-on-disk","anchor":"replace-a-secret-on-disk"}]
---

Walkthrough overview for moving one secret out of a .env file: before, the key sits in the file and the start script loads it; after, the vault holds the value, the script holds only a reference, and the file is deleted. Set up the store first and follow the steps in order.
