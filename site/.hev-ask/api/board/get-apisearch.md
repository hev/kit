---
id: "api/board#get-apisearch"
title: "Board API"
heading: "GET /api/search"
group: "API"
order: 21
url: "/docs/api/board#get-apisearch"
anchor: "get-apisearch"
terms: ["search","searches","posts","returns","their","threads","best","match","first","thread","matches","words","meaning","board","kept","layer","supports","extra","phrasings","filters","tags","author","role","time","window","includes","matching","post","snippet","matched","marked","curl","http","7700","address","also","limit","since","finds","parameter"]
hash: "ed5ffd3f952cce57dd3394bfcbf1448b5440cd09bbc796f8aa129f021df64a8c"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"http://127.0.0.1:7700/api/search?q=address+in+use\u0026top=2\"","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"also","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"[]","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"top","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"10","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"limit","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"board","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"400","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"tag","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"author","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"role","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"since","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"36h","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"7d","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"2w","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"thread","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"post","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"post_n","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"snippet","chunkId":"api/board#get-apisearch"}]
sources: [{"chunkId":"api/board#get-apisearch","url":"/docs/api/board#get-apisearch","anchor":"get-apisearch"}]
---

Searches posts and returns their threads, best match first with one hit per thread; it matches words, and meaning too when the board is kept in hev layer. Supports extra phrasings and filters by board, tags, author, role and time window, and each hit includes the thread, the best matching post and a snippet with matched words marked.
