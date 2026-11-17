---
id: "secrets#3-swap-the-file-for-the-reference"
title: "Secrets"
heading: "3. Swap the file for the reference"
group: "Operate"
order: 253
url: "/docs/secrets#3-swap-the-file-for-the-reference"
anchor: "3-swap-the-file-for-the-reference"
terms: ["swap","file","reference","third","step","replacing","secret","disk","change","start","script","launch","program","through","secrets","runner","values","exist","only","child","environment","settings","stay","plain","assignments","shell","needs","variables","itself","should","load","never","profile","after","value","exec","stripe","azkv","vault","server"]
hash: "31dd5477cffcdfa44cd3bc0f07b3289c8a0843b9ab0131fd242bda81dc245975"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# run.sh (after): a reference, not a value\nexec hev secrets run \\\n  --ref STRIPE_API_KEY=azkv://VAULT/stripe-api-key \\\n  --ttl STRIPE_API_KEY=1h \\\n  -- ./server","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":".env","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"hev secrets run","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"--ref","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"--ttl","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"op://","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"azkv://","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"eval \"$(hev secrets env --ref STRIPE_API_KEY=azkv://VAULT/stripe-api-key --ttl STRIPE_API_KEY=1h)\"","chunkId":"secrets#3-swap-the-file-for-the-reference"}]
sources: [{"chunkId":"secrets#3-swap-the-file-for-the-reference","url":"/docs/secrets#3-swap-the-file-for-the-reference","anchor":"3-swap-the-file-for-the-reference"}]
---

Third step of replacing a secret on disk: change the start script to launch the program through the secrets runner with a reference and TTL per secret, so values exist only in the child's environment. Non-secret settings stay as plain assignments, and a shell that needs the variables itself should load them in the job that needs them, never in a shell profile.
