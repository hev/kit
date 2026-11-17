---
id: "api#auth"
title: "API overview"
heading: "Auth"
group: "API"
order: 10
url: "/docs/api#auth"
anchor: "auth"
terms: ["auth","local","dashboard","board","servers","authentication","anything","reach","their","port","call","should","stay","localhost","trusted","network","club","uses","bearer","saved","sign","belongs","person","machine","revoked","logging","serve","8787","bind","addr","authorization","login","logout","none","listens","unless","pass","address","give","keep"]
hash: "61ec49d7119b55524beb5c4249663649b7f2018eb088f853dcd090bbb002b591"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev serve","chunkId":"api#auth"},{"kind":"code","literal":"hev board serve","chunkId":"api#auth"},{"kind":"code","literal":"127.0.0.1:8787","chunkId":"api#auth"},{"kind":"code","literal":"--bind","chunkId":"api#auth"},{"kind":"code","literal":"--port","chunkId":"api#auth"},{"kind":"code","literal":"--addr","chunkId":"api#auth"},{"kind":"code","literal":"[board] addr","chunkId":"api#auth"},{"kind":"code","literal":"Authorization: Bearer hk_…","chunkId":"api#auth"},{"kind":"code","literal":"hev login","chunkId":"api#auth"},{"kind":"code","literal":"hev logout","chunkId":"api#auth"}]
sources: [{"chunkId":"api#auth","url":"/docs/api#auth","anchor":"auth"}]
---

The local dashboard and board servers have no authentication, so anything that can reach their port can call them and they should stay on localhost or a trusted network. The Club Kit API uses a bearer kit key saved at sign-in, which belongs to one person on one machine and is revoked by logging out.
