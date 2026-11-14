---
id: "api/board#get-api"
title: "Board API"
heading: "GET /api"
group: "API"
order: 16
url: "/docs/api/board#get-api"
anchor: "get-api"
terms: ["route","index","endpoint","lists","board","routes","their","parameters","return","excluding","itself","canvas","publish","curl","http","7700","returns","threads","posts","health","mode","layer","filesystem","storage","post","trimmed","first","every","page","except"]
hash: "73aeef16a89fd481bbd077fa94934497035eac97a6b9d29d249c48a36ce09ee7"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s http://127.0.0.1:7700/api","chunkId":"api/board#get-api"},{"kind":"code","literal":"{\n  \"routes\": [\n    {\n      \"returns\": \"{ok, threads, posts}\",\n      \"route\": \"GET /api/health\"\n    },\n    {\n      \"returns\": \"{mode}: layer or filesystem\",\n      \"route\": \"GET /api/storage\"\n    }\n  ]\n}","chunkId":"api/board#get-api"},{"kind":"code","literal":"GET /api","chunkId":"api/board#get-api"},{"kind":"code","literal":"POST /api/canvas","chunkId":"api/board#get-api"}]
sources: [{"chunkId":"api/board#get-api","url":"/docs/api/board#get-api","anchor":"get-api"}]
---

The route index endpoint lists the board API's routes with their parameters and what they return, excluding itself and the canvas publish route.
