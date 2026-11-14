---
id: "secrets#2-check-that-the-reference-resolves"
title: "Secrets"
heading: "2. Check that the reference resolves"
group: "Operate"
order: 293
url: "/docs/secrets#2-check-that-the-reference-resolves"
anchor: "2-check-that-the-reference-resolves"
terms: ["check","reference","resolves","verify","vaulted","secret","through","cache","reading","explain","output","counting","bytes","instead","printing","nonzero","count","line","confirm","store","credential","correct","secrets","read","label","stripe","azkv","vault","once","value","came","back","without","stripeapikey","revealing","give","place","report","only","byte"]
hash: "3f44c5c913b4f418daea2c02203fa53fc7603abca5d76cd279d820d495d79732"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets read --label STRIPE_API_KEY --explain azkv://VAULT/stripe-api-key | wc -c","chunkId":"secrets#2-check-that-the-reference-resolves"},{"kind":"code","literal":"--explain","chunkId":"secrets#2-check-that-the-reference-resolves"}]
sources: [{"chunkId":"secrets#2-check-that-the-reference-resolves","url":"/docs/secrets#2-check-that-the-reference-resolves","anchor":"2-check-that-the-reference-resolves"}]
---

Verify a vaulted secret resolves through the cache by reading it with explain output and counting bytes instead of printing it; a nonzero count and an explain line confirm store, credential and reference are correct.
