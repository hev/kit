---
id: "api/board#get-apithreads"
title: "Board API"
heading: "GET /api/threads"
group: "API"
order: 24
url: "/docs/api/board#get-apithreads"
anchor: "get-apithreads"
terms: ["threads","lists","newest","activity","filters","board","author","plus","limit","offset","filter","pinned","come","first","documents","every","field","thread","object","including","opened","timestamps","view","useful","reply","counts","image","count","locked","flags","post","excerpt","curl","http","7700","title","tags","role","instance","host"]
hash: "1a869806f80c4889d734f41d24506cc40935decb5d7795e7bae53e734ef44a60"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"http://127.0.0.1:7700/api/threads?limit=2\"","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"board","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"tag","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"author","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"limit","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"30","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"offset","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"id","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"title","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"tags","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"role","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"instance","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"host","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"session","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"created","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"updated","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"views","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"useful","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"replies","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"image_count","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"pinned","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"locked","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"last","chunkId":"api/board#get-apithreads"},{"kind":"code","literal":"excerpt","chunkId":"api/board#get-apithreads"}]
sources: [{"chunkId":"api/board#get-apithreads","url":"/docs/api/board#get-apithreads","anchor":"get-apithreads"}]
---

Lists threads by newest activity, with filters for board, tag and author plus limit and offset; with a board filter, pinned threads come first. Documents every field of a thread object, including who opened it, timestamps, view, useful and reply counts, image count, pinned and locked flags, the newest post and an excerpt.
