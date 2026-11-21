---
id: "secrets#replace-a-secret-on-disk"
title: "Secrets"
heading: "Replace a secret on disk"
group: "Operate"
order: 384
url: "/docs/secrets#replace-a-secret-on-disk"
anchor: "replace-a-secret-on-disk"
terms: ["replace","secret","disk","shows","replacing","plaintext","value","file","reference","under","same","name","names","vault","stripe","live","example","host","secrets","item","title","field","password","version","latest","azkv","server","export","single","quoted","double","eval","read","label","explain","keep","change","holds","comment","line"]
hash: "3a57a335ec9737b8d0e4f959652f33ff4271f78ffbf3d15d3d8b8c28a7bd838f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# .env\n# STRIPE_API_KEY=sk_live_EXAMPLE_VALUE\nSTRIPE_API_KEY=secret://stripe-api-key","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"Vault:     VAULT (the host's [secrets] vault)\nItem:      stripe-api-key (the item's title)\nField:     password (or the host's [secrets] field)\nReference: op://VAULT/stripe-api-key/password","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"Vault:     VAULT (the host's [secrets] vault)\nSecret:    stripe-api-key\nVersion:   latest\nReference: azkv://VAULT/stripe-api-key","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"hev secrets run --env-file .env -- ./server","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":".env","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"stripe-api-key","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"hev secrets run","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"secret://","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"op://","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"azkv://","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"export","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"'single quoted'","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"\"double quoted\"","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"--ref NAME=REF","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"--ttl NAME=1h","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"eval \"$(hev secrets env --env-file .env)\"","chunkId":"secrets#replace-a-secret-on-disk"},{"kind":"code","literal":"hev secrets read --label STRIPE_API_KEY --explain secret://stripe-api-key | wc -c","chunkId":"secrets#replace-a-secret-on-disk"}]
sources: [{"chunkId":"secrets#replace-a-secret-on-disk","url":"/docs/secrets#replace-a-secret-on-disk","anchor":"replace-a-secret-on-disk"}]
---

Shows replacing a plaintext value in a .env file with a secret:// reference under the same name, so the file names the secret and not its vault or value.
