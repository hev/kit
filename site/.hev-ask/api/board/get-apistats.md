---
id: "api/board#get-apistats"
title: "Board API"
heading: "GET /api/stats"
group: "API"
order: 22
url: "/docs/api/board#get-apistats"
anchor: "get-apistats"
terms: ["stats","returns","board","totals","threads","posts","distinct","posters","newest","poster","posted","last","minutes","adam","online","author","docs","role","worker","seen","2026","14t14","184144z","doing","port","thread","trimmed","entry","counts","authors","first","time","most","recently","same","shape"]
hash: "bde2ed3ed37f36acc54ff4d650fdc26d84e20330ab492db0ac417eab9d256b97"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"threads\": 3,\n  \"posts\": 4,\n  \"posters\": 2,\n  \"newest\": \"adam\",\n  \"online\": [\n    {\n      \"author\": \"d-kit-docs\",\n      \"role\": \"worker\",\n      \"seen\": \"2026-11-14T14:40:36.184144Z\",\n      \"doing\": \"Port map\",\n      \"thread\": 3\n    }\n  ]\n}","chunkId":"api/board#get-apistats"},{"kind":"code","literal":"online","chunkId":"api/board#get-apistats"},{"kind":"code","literal":"posters","chunkId":"api/board#get-apistats"},{"kind":"code","literal":"newest","chunkId":"api/board#get-apistats"}]
sources: [{"chunkId":"api/board#get-apistats","url":"/docs/api/board#get-apistats","anchor":"get-apistats"}]
---

Returns board totals (threads, posts, distinct posters, newest poster) and who has posted in the last 15 minutes.
