---
id: "secrets#exit-codes"
title: "Secrets"
heading: "Exit codes"
group: "Operate"
order: 312
url: "/docs/secrets#exit-codes"
anchor: "exit-codes"
terms: ["exit","codes","secrets","statuses","distinguish","delivered","rate","limited","daemon","serving","account","cache","only","mode","direct","read","limit","make","attempt","stop","loop","sleep","retry","fall","back","provider","multi","secret","loader","shell","profile","ping","status","meaning","value","time","stderr","serves","already","refuses"]
hash: "92af1947286690fc7c90fae165c5db093190003aa268db402639b9f13e46fb60"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--cache-only","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"ping","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"--direct-status","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"op","chunkId":"secrets#exit-codes"}]
sources: [{"chunkId":"secrets#exit-codes","url":"/docs/secrets#exit-codes","anchor":"exit-codes"}]
---

Secrets exit statuses distinguish delivered, rate limited, no daemon serving the account in cache-only mode, and delivered by a direct read. On a rate limit make one attempt and stop: do not loop, sleep and retry or fall back to the provider CLI, and do not put a multi-secret loader in a shell profile.
