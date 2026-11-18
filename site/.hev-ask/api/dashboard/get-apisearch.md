---
id: "api/dashboard#get-apisearch"
title: "Dashboard API"
heading: "GET /api/search"
group: "API"
order: 43
url: "/docs/api/dashboard#get-apisearch"
anchor: "get-apisearch"
terms: ["search","searches","transcripts","grades","traces","matching","filters","returns","best","trace","hybrid","mode","ranks","relevance","phrase","matches","exact","file","archive","words","only","while","layer","adds","meaning","grade","result","session","summary","plus","snippet","occurred","score","storage","object","reports","curl","http","8787","flaky"]
hash: "6141b871abaf3c90afca226c3a06c984383b663e02e59179cae3c254915d9097"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s 'http://127.0.0.1:8787/api/search?q=flaky\u0026top=3'","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"{\"error\":\"q must contain 1–8192 bytes\",\"timing\":{\"layer_ms\":0,\"queries\":0,\"rows\":0}}","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"mode","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"hybrid","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"phrase","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"top","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"10000","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"50","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"window","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"candidate_limit","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"max(4 × top, 200)","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"candidate_truncated","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"transcript","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"eval","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"true","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"sessions","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"/api/sessions","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"snippet","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"turn_uuid","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"role","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"block_type","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"tool_name","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"hit_ts","chunkId":"api/dashboard#get-apisearch"},{"kind":"code","literal":"sidechain","chunkId":"api/dashboard#get-apisearch"}]
sources: [{"chunkId":"api/dashboard#get-apisearch","url":"/docs/api/dashboard#get-apisearch","anchor":"get-apisearch"}]
---

Searches the transcripts, and grades, of traces matching the filters and returns the best hit per trace. Hybrid mode ranks by relevance and phrase mode matches an exact phrase; with the file archive hybrid matches words only, while hev layer adds meaning and grade search. Each result is a session summary plus the matching snippet, where it occurred and its score, and a storage object reports what search can do on this archive.
