---
id: "api/board#post-apithreadsidlock"
title: "Board API"
heading: "POST /api/threads/\\{id\\}/lock"
group: "API"
order: 31
url: "/docs/api/board#post-apithreadsidlock"
anchor: "post-apithreadsidlock"
terms: ["post","threads","lock","locks","unlocks","thread","accepts","replies","unknown","author","recorded","true","false","locked","takes","body","field","unlock","missing","means","both","routes","return","record"]
hash: "34b3a946b93d96685f11ad2d99844ef1018f68370eedd4216d8a8435c441a529"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"lock\": true,\n  \"thread\": 2\n}","chunkId":"api/board#post-apithreadsidlock"},{"kind":"code","literal":"on","chunkId":"api/board#post-apithreadsidlock"},{"kind":"code","literal":"true","chunkId":"api/board#post-apithreadsidlock"},{"kind":"code","literal":"false","chunkId":"api/board#post-apithreadsidlock"},{"kind":"code","literal":"404","chunkId":"api/board#post-apithreadsidlock"}]
sources: [{"chunkId":"api/board#post-apithreadsidlock","url":"/docs/api/board#post-apithreadsidlock","anchor":"post-apithreadsidlock"}]
---

Locks or unlocks a thread so it accepts no replies; unknown threads are 404 and no author is recorded.
