---
id: "secrets#3-swap-the-file-for-the-reference"
title: "Secrets"
heading: "3. Swap the file for the reference"
group: "Operate"
order: 297
url: "/docs/secrets#3-swap-the-file-for-the-reference"
anchor: "3-swap-the-file-for-the-reference"
terms: ["swap","file","reference","replace","start","script","loading","secrets","runner","resolves","launches","program","values","only","environment","secret","keep","settings","plain","assignments","shell","needs","variables","evaluate","output","inside","rather","profile","after","value","exec","stripe","azkv","vault","server","eval","line","loads","starts","stripeapikey"]
hash: "272684604d8fe655978abe512e8b8cb422a1dbcde31e053ac0f577a38987810f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# run.sh (after): a reference, not a value\nexec hev secrets run \\\n  --ref STRIPE_API_KEY=azkv://VAULT/stripe-api-key \\\n  --ttl STRIPE_API_KEY=1h \\\n  -- ./server","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":".env","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"hev secrets run","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"--ref","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"--ttl","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"op://","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"azkv://","chunkId":"secrets#3-swap-the-file-for-the-reference"},{"kind":"code","literal":"eval \"$(hev secrets env --ref STRIPE_API_KEY=azkv://VAULT/stripe-api-key --ttl STRIPE_API_KEY=1h)\"","chunkId":"secrets#3-swap-the-file-for-the-reference"}]
sources: [{"chunkId":"secrets#3-swap-the-file-for-the-reference","url":"/docs/secrets#3-swap-the-file-for-the-reference","anchor":"3-swap-the-file-for-the-reference"}]
---

Replace the start script's .env loading with the secrets runner, which resolves each reference and launches the program with values only in its environment. Add one reference and TTL per secret, keep non-secret settings as plain assignments, and for a shell that needs variables, evaluate the env output inside the job rather than a profile.
