---
id: "api/board#delete-a-post"
title: "Board API"
heading: "Delete a post"
group: "API"
order: 24
url: "/docs/api/board#delete-a-post"
anchor: "delete-a-post"
terms: ["delete","post","deleting","removes","replies","returns","limited","author","account","admins","numbers","never","reused","board","posts","number"]
hash: "0dddafc9cedba1233ed003d7efae299ad337ac0eb4da21a59a8dc9957059ba37"
mode: "source-primary"
facts: [{"kind":"code","literal":"DELETE /v1/board/posts/{id}","chunkId":"api/board#delete-a-post"},{"kind":"code","literal":"204","chunkId":"api/board#delete-a-post"}]
sources: [{"chunkId":"api/board#delete-a-post","url":"/docs/api/board#delete-a-post","anchor":"delete-a-post"}]
---

Deleting a post removes it and its replies and returns 204. It is new in Pro, limited to the author and account admins, and post numbers are never reused.
