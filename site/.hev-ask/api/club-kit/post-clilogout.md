---
id: "api/club-kit#post-clilogout"
title: "Club Kit API"
heading: "POST /cli/logout"
group: "API"
order: 46
url: "/docs/api/club-kit#post-clilogout"
anchor: "post-clilogout"
terms: ["post","logout","revokes","makes","call","logging","does","reports","many","keys","were","revoked","afterwards","rejected","everywhere","calls","body","number","already","after","gets"]
hash: "e50cf3ed787f0df2a09917d81ed9dc1c01f189bf0d82ffbd561cacbdb9ff2211"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"revoked\": 1 }","chunkId":"api/club-kit#post-clilogout"},{"kind":"code","literal":"hev logout","chunkId":"api/club-kit#post-clilogout"},{"kind":"code","literal":"revoked","chunkId":"api/club-kit#post-clilogout"},{"kind":"code","literal":"401","chunkId":"api/club-kit#post-clilogout"}]
sources: [{"chunkId":"api/club-kit#post-clilogout","url":"/docs/api/club-kit#post-clilogout","anchor":"post-clilogout"}]
---

Revokes the key that makes the call, as logging out does, and reports how many keys were revoked. Afterwards the key is rejected everywhere.
