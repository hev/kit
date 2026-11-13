---
id: "secrets#why-not-a-env-file"
title: "Secrets"
heading: "Why not a .env file"
group: "Operate"
order: 274
url: "/docs/secrets#why-not-a-env-file"
anchor: "why-not-a-env-file"
terms: ["file","explains","plaintext","risky","coding","agents","work","read","leak","context","shared","traces","gets","copied","worktrees","hosts","containers","backups","commits","every","host","needs","copy","rarely","rotated","nothing","records","vault","reference","secret","live","scripts","while","value","fetched","start","rotation","change","real","request"]
hash: "9770e5dd36804e7ba5a082e9f492c1a7c43601b604ed3755831ce33d96c871ad"
mode: "agent-primary"
facts: [{"kind":"code","literal":".env","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"cat","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"grep","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"rsync","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":".gitignore","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"azkv://vault/secret","chunkId":"secrets#why-not-a-env-file"}]
sources: [{"chunkId":"secrets#why-not-a-env-file","url":"/docs/secrets#why-not-a-env-file","anchor":"why-not-a-env-file"}]
---

Explains why a plaintext .env file is risky where coding agents work: agents can read it and leak it into context and shared traces, it gets copied into worktrees, hosts, containers, backups and commits, every host needs its own copy so it rarely gets rotated, and nothing records who used it. A vault reference is not a secret, so it can live in scripts and commits while the value is fetched at start, rotation is one vault change, and every real request is logged; it removes the copy at rest but not the value from the program using it.
