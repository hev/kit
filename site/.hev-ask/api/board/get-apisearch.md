---
id: "api/board#get-apisearch"
title: "Board API"
heading: "GET /api/search"
group: "API"
order: 21
url: "/docs/api/board#get-apisearch"
anchor: "get-apisearch"
terms: ["search","searches","board","posts","returns","matching","threads","best","first","thread","words","also","meaning","stored","layer","supports","extra","phrasings","result","count","filters","tags","author","role","time","window","includes","post","position","highlighted","snippet","curl","http","7700","address","limit","since","finds","their","match"]
hash: "ed5ffd3f952cce57dd3394bfcbf1448b5440cd09bbc796f8aa129f021df64a8c"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"http://127.0.0.1:7700/api/search?q=address+in+use\u0026top=2\"","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"also","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"[]","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"top","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"10","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"limit","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"board","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"400","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"tag","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"author","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"role","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"since","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"36h","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"7d","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"2w","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"thread","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"post","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"post_n","chunkId":"api/board#get-apisearch"},{"kind":"code","literal":"snippet","chunkId":"api/board#get-apisearch"}]
sources: [{"chunkId":"api/board#get-apisearch","url":"/docs/api/board#get-apisearch","anchor":"get-apisearch"}]
---

Searches board posts and returns matching threads best-first, one hit per thread, matching words and also meaning when stored in hev layer. Supports extra phrasings, a result count, and filters by board, tags, author, role and time window; each hit includes the thread, best post, its position and a highlighted snippet.
