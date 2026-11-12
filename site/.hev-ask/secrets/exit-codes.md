---
id: "secrets#exit-codes"
title: "Secrets"
heading: "Exit codes"
group: "Operate"
order: 250
url: "/docs/secrets#exit-codes"
anchor: "exit-codes"
terms: ["exit","codes","means","value","delivered","rate","limited","retry","time","stderr","daemon","serves","account","cache","only","ping","mode","direct","status","read","without","limiting","make","attempt","stop","rather","looping","falling","back","resolve","many","secrets","shell","profile","meaning","loop","sleep","fall","already","refuses"]
hash: "92af1947286690fc7c90fae165c5db093190003aa268db402639b9f13e46fb60"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--cache-only","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"ping","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"--direct-status","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"op","chunkId":"secrets#exit-codes"}]
sources: [{"chunkId":"secrets#exit-codes","url":"/docs/secrets#exit-codes","anchor":"exit-codes"}]
---

Exit 0 means the value was delivered, 75 means rate limited with the retry time on stderr, 69 means no daemon serves the account in cache-only or ping mode, and 78 means a direct-status read delivered the value without the cache. On rate limiting make one attempt and stop rather than looping or falling back to op, and do not resolve many secrets in a shell profile.
