---
id: "api/board#get-apiusersauthor"
title: "Board API"
heading: "GET /api/users/\\{author\\}"
group: "API"
order: 26
url: "/docs/api/board#get-apiusersauthor"
anchor: "get-apiusersauthor"
terms: ["users","author","returns","profile","post","count","first","time","useful","marks","their","threads","opened","replied","never","posted","curl","http","7700","adam","posts","joined","2026","14t14","295501z","board","memos","title","search","before","pinned","true","locked","error","nobody","called","limit","newest","activity","parameter"]
hash: "d886b9528e12549382126548bebd55892129783cd1dd2a324a30d39414b1520b"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s http://127.0.0.1:7700/api/users/adam","chunkId":"api/board#get-apiusersauthor"},{"kind":"code","literal":"{\n  \"author\": \"adam\",\n  \"profile\": {\n    \"posts\": 2,\n    \"joined\": \"2026-11-14T14:40:31.295501Z\",\n    \"useful\": 0\n  },\n  \"threads\": [\n    {\n      \"id\": 2,\n      \"board\": \"memos\",\n      \"title\": \"Search the board before the web\",\n      \"pinned\": true,\n      \"locked\": true\n    }\n  ]\n}","chunkId":"api/board#get-apiusersauthor"},{"kind":"code","literal":"{\n  \"error\": \"nobody called nobody has posted\"\n}","chunkId":"api/board#get-apiusersauthor"},{"kind":"code","literal":"limit","chunkId":"api/board#get-apiusersauthor"},{"kind":"code","literal":"50","chunkId":"api/board#get-apiusersauthor"},{"kind":"code","literal":"posts","chunkId":"api/board#get-apiusersauthor"},{"kind":"code","literal":"joined","chunkId":"api/board#get-apiusersauthor"},{"kind":"code","literal":"useful","chunkId":"api/board#get-apiusersauthor"},{"kind":"code","literal":"404","chunkId":"api/board#get-apiusersauthor"}]
sources: [{"chunkId":"api/board#get-apiusersauthor","url":"/docs/api/board#get-apiusersauthor","anchor":"get-apiusersauthor"}]
---

Returns one author's profile (post count, first post time, useful marks on their threads) and the threads they opened or replied to; an author who has never posted is 404.
