---
id: "secrets#why-not-a-env-file"
title: "Secrets"
heading: "Why not a .env file"
group: "Operate"
order: 331
url: "/docs/secrets#why-not-a-env-file"
anchor: "why-not-a-env-file"
terms: ["file","plain","keeps","real","secrets","coding","agents","work","agent","read","context","transcripts","gets","copied","worktrees","hosts","containers","backups","every","host","needs","copy","rotation","skipped","nothing","records","reference","secret","scripts","commits","while","value","stays","vault","memory","removes","rest","does","hide","program"]
hash: "bdc7f5eb0a3798ff6681ed7e2587f9a262c0cdd9b5dc7f036c60f4e7f2c8cc21"
mode: "agent-primary"
facts: [{"kind":"code","literal":".env","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"cat","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"grep","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"rsync","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":".gitignore","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"secret://stripe-api-key","chunkId":"secrets#why-not-a-env-file"}]
sources: [{"chunkId":"secrets#why-not-a-env-file","url":"/docs/secrets#why-not-a-env-file","anchor":"why-not-a-env-file"}]
---

A plain .env file keeps real secrets where coding agents work: an agent can read it into its context and transcripts, it gets copied into worktrees, hosts, containers and backups, every host needs its own copy so rotation is skipped, and nothing records who used it. A reference is not a secret, so it can sit in scripts or commits while the value stays in the vault and in memory; it removes the copy at rest but does not hide the value from the program that uses it.
