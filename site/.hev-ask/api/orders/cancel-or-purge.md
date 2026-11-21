---
id: "api/orders#cancel-or-purge"
title: "Orders API"
heading: "Cancel or purge"
group: "API"
order: 45
url: "/docs/api/orders#cancel-or-purge"
anchor: "cancel-or-purge"
terms: ["cancel","purge","delete","dequeues","queued","order","asks","live","stop","state","halting","record","stays","halted","ended","directory","worktree","removed","pushed","branches","untouched","orders","true","readable","later","reads","branch","open","left","alone"]
hash: "d85f4f29174c569488e9ee5d6029979b061a6ace6ee206edb914880279de02d7"
mode: "source-primary"
facts: [{"kind":"code","literal":"DELETE /v1/orders/{id}","chunkId":"api/orders#cancel-or-purge"},{"kind":"code","literal":"204","chunkId":"api/orders#cancel-or-purge"},{"kind":"code","literal":"202","chunkId":"api/orders#cancel-or-purge"},{"kind":"code","literal":"halting","chunkId":"api/orders#cancel-or-purge"},{"kind":"code","literal":"halted","chunkId":"api/orders#cancel-or-purge"},{"kind":"code","literal":"?purge=true","chunkId":"api/orders#cancel-or-purge"},{"kind":"code","literal":"404","chunkId":"api/orders#cancel-or-purge"}]
sources: [{"chunkId":"api/orders#cancel-or-purge","url":"/docs/api/orders#cancel-or-purge","anchor":"cancel-or-purge"}]
---

Delete dequeues a queued order or asks a live run to stop (state halting); the record stays as halted. With purge, an ended run's directory and worktree are removed; pushed branches and PRs are untouched.
