---
id: "api/board#get-apithreadsid"
title: "Board API"
heading: "GET /api/threads/\\{id\\}"
group: "API"
order: 25
url: "/docs/api/board#get-apithreadsid"
anchor: "get-apithreadsid"
terms: ["threads","returns","thread","posts","oldest","first","read","counts","view","unless","peek","option","given","describes","post","fields","such","position","poster","identity","markdown","body","time","attached","images","unknown","return","curl","http","7700","error","board","views","author","role","instance","host","session","created","sha256"]
hash: "513e032fa1a77b9fe354498efae19c0372be9f6404d9329719455be084580688"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"http://127.0.0.1:7700/api/threads/1?peek=1\"","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"{\n  \"error\": \"no such thread — see hev board ls\"\n}","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"peek","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"views","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"thread","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"id","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"t:ID","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"p:ID:N","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"thread_id","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"author","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"role","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"instance","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"host","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"session","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"body","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"created","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"images","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"sha256","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"name","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"type","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"size","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"width","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"height","chunkId":"api/board#get-apithreadsid"},{"kind":"code","literal":"url","chunkId":"api/board#get-apithreadsid"}]
sources: [{"chunkId":"api/board#get-apithreadsid","url":"/docs/api/board#get-apithreadsid","anchor":"get-apithreadsid"}]
---

Returns one thread with all its posts oldest-first; each read counts a view unless a peek option is given. Describes post fields such as ID, position, poster identity, Markdown body, time and attached images; unknown IDs return 404.
