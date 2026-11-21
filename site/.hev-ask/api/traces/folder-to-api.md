---
id: "api/traces#folder-to-api"
title: "Traces API"
heading: "Folder to API"
group: "API"
order: 63
url: "/docs/api/traces#folder-to-api"
anchor: "folder-to-api"
terms: ["folder","maps","archive","sessions","blocks","chunks","evals","folders","matching","routes","shard","json","sessionrow","post","traces","delete","jsonl","blockrow","search","blobs","sha256","images","index","blockrows","returned","session","detail","rows","read","only","through","searched","newest","first","listing","list","route","ordering"]
hash: "d3bdb4401e53833a2149a5c98d5449e86a5915e6eb81b32e508527c03ee29219"
mode: "source-primary"
facts: [{"kind":"code","literal":"sessions/\u003cshard\u003e/\u003ckey\u003e.json","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"SessionRow","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"POST","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"GET /v1/traces/sessions","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"GET","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"DELETE /v1/traces/sessions/{id}","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"blocks/\u003cshard\u003e/\u003ckey\u003e.jsonl","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"BlockRow","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"POST /v1/traces/blocks","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"chunks/\u003cshard\u003e/\u003ckey\u003e.jsonl","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"POST /v1/traces/chunks","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"evals.jsonl","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"/v1/traces/search","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"blobs/sha256/…","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"PUT","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"GET /v1/traces/images/{sha256}","chunkId":"api/traces#folder-to-api"},{"kind":"code","literal":"index.jsonl","chunkId":"api/traces#folder-to-api"}]
sources: [{"chunkId":"api/traces#folder-to-api","url":"/docs/api/traces#folder-to-api","anchor":"folder-to-api"}]
---

Maps the OSS archive's sessions, blocks, chunks and evals folders to the matching API routes.
