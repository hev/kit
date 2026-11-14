---
id: "secrets#4-delete-the-file-and-rotate-the-key"
title: "Secrets"
heading: "4. Delete the file and rotate the key"
group: "Operate"
order: 299
url: "/docs/secrets#4-delete-the-file-and-rotate-the-key"
anchor: "4-delete-the-file-and-rotate-the-key"
terms: ["delete","file","rotate","after","script","works","ensure","ignored","check","history","issuer","store","value","under","same","vault","name","since","should","treated","exposed","hosts","pick","expires","immediately","zero","oneline","output","means","never","committed","stripe","program","once","finish","moving","project","first","confirm","start"]
hash: "32a5a8f9d40a6ac02081ad32858962cc910758b3fb5d49145150273f929e9ef8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"rm .env\ngit log --oneline --all -- .env   # no output means it was never committed","chunkId":"secrets#4-delete-the-file-and-rotate-the-key"},{"kind":"code","literal":".env","chunkId":"secrets#4-delete-the-file-and-rotate-the-key"},{"kind":"code","literal":"--ttl STRIPE_API_KEY=0s","chunkId":"secrets#4-delete-the-file-and-rotate-the-key"}]
sources: [{"chunkId":"secrets#4-delete-the-file-and-rotate-the-key","url":"/docs/secrets#4-delete-the-file-and-rotate-the-key","anchor":"4-delete-the-file-and-rotate-the-key"}]
---

After the new script works, delete the .env file, ensure it is ignored, and check git history for it; then rotate the key at its issuer and store the new value under the same vault name, since the old one should be treated as exposed. Hosts pick up the new value when the TTL expires or immediately with a zero TTL.
