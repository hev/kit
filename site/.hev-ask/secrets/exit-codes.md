---
id: "secrets#exit-codes"
title: "Secrets"
heading: "Exit codes"
group: "Operate"
order: 260
url: "/docs/secrets#exit-codes"
anchor: "exit-codes"
terms: ["exit","codes","lists","secrets","commands","success","rate","limited","retry","time","stderr","daemon","serving","account","cache","only","ping","mode","delivered","direct","read","rather","limit","make","attempt","stop","looping","falling","back","1password","never","loader","resolves","many","shell","profile","status","meaning","value","serves"]
hash: "92af1947286690fc7c90fae165c5db093190003aa268db402639b9f13e46fb60"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--cache-only","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"ping","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"--direct-status","chunkId":"secrets#exit-codes"},{"kind":"code","literal":"op","chunkId":"secrets#exit-codes"}]
sources: [{"chunkId":"secrets#exit-codes","url":"/docs/secrets#exit-codes","anchor":"exit-codes"}]
---

Lists exit codes for secrets commands: success, rate limited with the retry time on stderr, no daemon serving the account in cache-only or ping mode, and delivered by a direct read rather than the cache. On a rate limit, make one attempt and stop rather than looping or falling back to the 1Password CLI, and never put a loader that resolves many secrets in a shell profile.
