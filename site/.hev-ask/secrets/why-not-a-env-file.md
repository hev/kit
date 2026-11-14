---
id: "secrets#why-not-a-env-file"
title: "Secrets"
heading: "Why not a .env file"
group: "Operate"
order: 318
url: "/docs/secrets#why-not-a-env-file"
anchor: "why-not-a-env-file"
terms: ["file","files","dangerous","coding","agents","read","value","ends","context","transcripts","shared","traces","copies","spread","through","worktrees","syncs","builds","backups","commits","every","host","needs","copy","rotation","neglected","reads","leave","audit","trail","vault","reference","secret","live","scripts","orders","while","stays","daemon","memory"]
hash: "d9d9ae783dbeebe2280eb70ee548cbb4c888b98325201eabb66850f85ec74afa"
mode: "agent-primary"
facts: [{"kind":"code","literal":".env","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"cat","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"grep","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"rsync","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":".gitignore","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"op://vault/item/field","chunkId":"secrets#why-not-a-env-file"},{"kind":"code","literal":"azkv://vault/secret","chunkId":"secrets#why-not-a-env-file"}]
sources: [{"chunkId":"secrets#why-not-a-env-file","url":"/docs/secrets#why-not-a-env-file","anchor":"why-not-a-env-file"}]
---

Why .env files are dangerous with coding agents: agents can read them and the value ends up in context, transcripts and shared traces; copies spread through worktrees, syncs, builds, backups and commits; every host needs its own copy so rotation is neglected; and reads leave no audit trail. A vault reference is not a secret, so it can live in scripts and orders while the value stays in the vault, the daemon's memory and the program's environment.
