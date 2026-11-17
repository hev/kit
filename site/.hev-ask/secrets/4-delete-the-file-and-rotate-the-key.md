---
id: "secrets#4-delete-the-file-and-rotate-the-key"
title: "Secrets"
heading: "4. Delete the file and rotate the key"
group: "Operate"
order: 255
url: "/docs/secrets#4-delete-the-file-and-rotate-the-key"
anchor: "4-delete-the-file-and-rotate-the-key"
terms: ["delete","file","rotate","final","step","replacing","secret","disk","once","script","works","check","history","confirm","never","committed","issuer","store","value","under","same","vault","name","since","should","treated","exposed","hosts","pick","their","expires","immediately","zero","oneline","output","means","stripe","program","agents","backups"]
hash: "464fd4c4991e063780174c34adb5ea9ef3e698ff78f198338d596492c08f9c8a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"rm .env\ngit log --oneline --all -- .env   # no output means it was never committed","chunkId":"secrets#4-delete-the-file-and-rotate-the-key"},{"kind":"code","literal":".env","chunkId":"secrets#4-delete-the-file-and-rotate-the-key"},{"kind":"code","literal":"--ttl STRIPE_API_KEY=0s","chunkId":"secrets#4-delete-the-file-and-rotate-the-key"}]
sources: [{"chunkId":"secrets#4-delete-the-file-and-rotate-the-key","url":"/docs/secrets#4-delete-the-file-and-rotate-the-key","anchor":"4-delete-the-file-and-rotate-the-key"}]
---

Final step of replacing a secret on disk: once the new script works, delete the .env file and check git history to confirm it was never committed. Then rotate the key at its issuer and store the new value under the same vault name, since the old value should be treated as exposed; hosts pick up the new value when their TTL expires, or immediately with a zero TTL.
