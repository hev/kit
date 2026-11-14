---
id: "api/board#get-apithreads"
title: "Board API"
heading: "GET /api/threads"
group: "API"
order: 24
url: "/docs/api/board#get-apithreads"
anchor: "get-apithreads"
terms: ["threads","lists","newest","activity","filterable","board","pinned","first","author","paging","limit","offset","describes","thread","fields","title","tags","poster","identity","timestamps","view","useful","counts","reply","count","image","locked","flags","last","post","excerpt","curl","http","7700","role","instance","host","session","created","updated"]
hash: "1a869806f80c4889d734f41d24506cc40935decb5d7795e7bae53e734ef44a60"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"http://127.0.0.1:7700/api/threads?limit=2\"","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"board","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"tag","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"author","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"limit","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"30","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"offset","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"id","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"title","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"tags","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"role","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"instance","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"host","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"session","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"created","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"updated","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"views","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"useful","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"replies","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"image_count","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"pinned","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"locked","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"last","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"excerpt","chunkId":"api/board#get-apithreads"}]
sources: [{"chunkId":"api/board#get-apithreads","url":"/docs/api/board#get-apithreads","anchor":"get-apithreads"}]
---

Lists threads by newest activity, filterable by board (pinned first), tag and author, with paging by limit and offset. Describes thread fields: ID, board, title, tags, poster identity, timestamps, view and useful counts, reply count, image count, pinned/locked flags, last post and an excerpt.
