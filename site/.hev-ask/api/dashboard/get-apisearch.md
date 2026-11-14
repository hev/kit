---
id: "api/dashboard#get-apisearch"
title: "Dashboard API"
heading: "GET /api/search"
group: "API"
order: 43
url: "/docs/api/dashboard#get-apisearch"
anchor: "get-apisearch"
terms: ["search","searches","transcripts","grades","layer","traces","matching","filters","returns","best","trace","modes","hybrid","ranking","exact","phrase","file","archive","matches","words","only","meaning","result","session","fields","plus","snippet","role","block","type","score","source","storage","object","explains","capabilities","curl","http","8787","flaky"]
hash: "6141b871abaf3c90afca226c3a06c984383b663e02e59179cae3c254915d9097"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s 'http://127.0.0.1:8787/api/search?q=flaky\u0026top=3'","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"{\"error\":\"q must contain 1–8192 bytes\",\"timing\":{\"layer_ms\":0,\"queries\":0,\"rows\":0}}","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"mode","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"hybrid","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"phrase","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"top","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"10000","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"50","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"window","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"candidate_limit","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"max(4 × top, 200)","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"candidate_truncated","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"transcript","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"eval","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"true","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"sessions","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"/api/sessions","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"snippet","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"turn_uuid","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"role","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"block_type","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"tool_name","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"hit_ts","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"sidechain","chunkId":"api/dashboard#get-apisearch"}]
sources: [{"chunkId":"api/dashboard#get-apisearch","url":"/docs/api/dashboard#get-apisearch","anchor":"get-apisearch"}]
---

Searches transcripts (and grades, with hev layer) of traces matching the filters and returns the best hit per trace. Modes are hybrid ranking or exact phrase; the file archive matches words only, hev layer matches meaning too. Each result has session fields plus the snippet, role, block type, score and source, and a storage object explains search capabilities.
