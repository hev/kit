---
id: "secrets#exit-codes"
title: "Secrets"
heading: "Exit codes"
group: "Operate"
order: 304
url: "/docs/secrets#exit-codes"
anchor: "exit-codes"
terms: ["exit","codes","secrets","success","rate","limited","retry","time","stderr","daemon","serving","account","cache","only","ping","mode","value","delivered","direct","read","limit","make","attempt","stop","never","loop","fall","back","multi","secret","loaders","shell","profiles","status","meaning","serves","sleep","already","refuses","send"]
hash: "92af1947286690fc7c90fae165c5db093190003aa268db402639b9f13e46fb60"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--cache-only","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"ping","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"--direct-status","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"op","chunkId":"secrets#exit-codes"}]
sources: [{"chunkId":"secrets#exit-codes","url":"/docs/secrets#exit-codes","anchor":"exit-codes"}]
---

Secrets exit codes: success, 75 for rate limited with retry time on stderr, 69 for no daemon serving the account in cache-only or ping mode, and 78 for a value delivered by direct read. On a rate limit make one attempt and stop, never loop or fall back, and don't put multi-secret loaders in shell profiles.
