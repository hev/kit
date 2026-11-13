---
id: "secrets#2-check-that-the-reference-resolves"
title: "Secrets"
heading: "2. Check that the reference resolves"
group: "Operate"
order: 249
url: "/docs/secrets#2-check-that-the-reference-resolves"
anchor: "2-check-that-the-reference-resolves"
terms: ["check","reference","resolves","second","step","replacing","secret","disk","read","once","through","cache","count","returned","bytes","without","printing","value","nonzero","plus","explain","line","stderr","confirms","store","credential","correct","secrets","label","stripe","azkv","vault","came","back","stripeapikey","above","zero","mean","right"]
hash: "693200051d557a6d5a6c7fa0e8c5e2ed2ab1c98529423d1b327e42f67c1288e4"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets read --label STRIPE_API_KEY --explain azkv://VAULT/stripe-api-key | wc -c","chunkId":"secrets#2-check-that-the-reference-resolves"},{"kind":"code","literal":"--explain","chunkId":"secrets#2-check-that-the-reference-resolves"}]
sources: [{"chunkId":"secrets#2-check-that-the-reference-resolves","url":"/docs/secrets#2-check-that-the-reference-resolves","anchor":"2-check-that-the-reference-resolves"}]
---

Second step of replacing a secret on disk: read the new reference once through the cache and count the returned bytes without printing the value. A nonzero count plus an explain line on stderr confirms the store, credential and reference are correct.
