---
id: "api/board#get-api"
title: "Board API"
heading: "GET /api"
group: "API"
order: 16
url: "/docs/api/board#get-api"
anchor: "get-api"
terms: ["route","index","endpoint","returns","list","board","routes","omits","itself","canvas","publishing","curl","http","7700","threads","posts","health","mode","layer","filesystem","storage","post","trimmed","first","lists","every","page","except"]
hash: "73aeef16a89fd481bbd077fa94934497035eac97a6b9d29d249c48a36ce09ee7"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s http://127.0.0.1:7700/api","chunkId":"api/board#get-api"},{"kind":"code","literal":"{\n  \"routes\": [\n    {\n      \"returns\": \"{ok, threads, posts}\",\n      \"route\": \"GET /api/health\"\n    },\n    {\n      \"returns\": \"{mode}: layer or filesystem\",\n      \"route\": \"GET /api/storage\"\n    }\n  ]\n}","chunkId":"api/board#get-api"},{"kind":"code","literal":"GET /api","chunkId":"api/board#get-api"},{"kind":"code","literal":"POST /api/canvas","chunkId":"api/board#get-api"}]
sources: [{"chunkId":"api/board#get-api","url":"/docs/api/board#get-api","anchor":"get-api"}]
---

The route index endpoint returns the list of board API routes with what each returns. It omits itself and the canvas publishing route.
