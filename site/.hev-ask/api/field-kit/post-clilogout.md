---
id: "api/field-kit#post-clilogout"
title: "Field Kit API"
heading: "POST /cli/logout"
group: "API"
order: 59
url: "/docs/api/field-kit#post-clilogout"
anchor: "post-clilogout"
terms: ["post","logout","revokes","calling","after","gets","everywhere","reports","many","keys","were","revoked","makes","call","calls","body","number","already"]
hash: "791d2eab421aa7d4c56806efb0d10e70ed1d15d9dbfe6667a7657b8d3da8cdf9"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"revoked\": 1 }","chunkId":"api/field-kit#post-clilogout"},{"kind":"code","literal":"hev logout","chunkId":"api/field-kit#post-clilogout"},{"kind":"code","literal":"revoked","chunkId":"api/field-kit#post-clilogout"},{"kind":"code","literal":"401","chunkId":"api/field-kit#post-clilogout"}]
sources: [{"chunkId":"api/field-kit#post-clilogout","url":"/docs/api/field-kit#post-clilogout","anchor":"post-clilogout"}]
---

Revokes the calling kit key, after which it gets 401 everywhere; reports how many keys were revoked.
