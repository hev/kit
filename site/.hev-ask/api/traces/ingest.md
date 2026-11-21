---
id: "api/traces#ingest"
title: "Traces API"
heading: "Ingest"
group: "API"
order: 65
url: "/docs/api/traces#ingest"
anchor: "ingest"
terms: ["ingest","three","routes","accept","batches","rows","kind","sessions","blocks","chunks","namespace","resolved","curl","https","hevmind","traces","authorization","bearer","content","type","application","json","019a","session","harness","claude","start","1763575622000","1763579222000","accepted","rejected","post","sessionrow","state","reverted","blockrow","chunkrow","text","index","error"]
hash: "d4153df0fd66acdd6ab4aa689b55ee08e5d8406c17a4885f16f4b3dd9aad1f9e"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s https://kit.hevmind.com/v1/traces/sessions \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"rows\":[{\"id\":\"s-019a…\",\"session_id\":\"019a…\",\"harness\":\"claude\",\"start\":1763575622000,\"end\":1763579222000}]}'","chunkId":"api/traces#ingest"},{"kind":"code","literal":"{ \"accepted\": 1, \"rejected\": [] }","chunkId":"api/traces#ingest"},{"kind":"code","literal":"rows","chunkId":"api/traces#ingest"},{"kind":"code","literal":"POST /v1/traces/sessions","chunkId":"api/traces#ingest"},{"kind":"code","literal":"SessionRow","chunkId":"api/traces#ingest"},{"kind":"code","literal":"id","chunkId":"api/traces#ingest"},{"kind":"code","literal":"pr_state","chunkId":"api/traces#ingest"},{"kind":"code","literal":"ci_state","chunkId":"api/traces#ingest"},{"kind":"code","literal":"reverted","chunkId":"api/traces#ingest"},{"kind":"code","literal":"POST /v1/traces/blocks","chunkId":"api/traces#ingest"},{"kind":"code","literal":"BlockRow","chunkId":"api/traces#ingest"},{"kind":"code","literal":"seq","chunkId":"api/traces#ingest"},{"kind":"code","literal":"POST /v1/traces/chunks","chunkId":"api/traces#ingest"},{"kind":"code","literal":"ChunkRow","chunkId":"api/traces#ingest"},{"kind":"code","literal":"text","chunkId":"api/traces#ingest"},{"kind":"code","literal":"rejected","chunkId":"api/traces#ingest"},{"kind":"code","literal":"index","chunkId":"api/traces#ingest"},{"kind":"code","literal":"error","chunkId":"api/traces#ingest"},{"kind":"code","literal":"Idempotency-Key","chunkId":"api/traces#ingest"},{"kind":"code","literal":"413","chunkId":"api/traces#ingest"},{"kind":"code","literal":"429","chunkId":"api/traces#ingest"},{"kind":"code","literal":"Retry-After","chunkId":"api/traces#ingest"},{"kind":"code","literal":"start","chunkId":"api/traces#ingest"},{"kind":"code","literal":"end","chunkId":"api/traces#ingest"}]
sources: [{"chunkId":"api/traces#ingest","url":"/docs/api/traces#ingest","anchor":"ingest"}]
---

Three routes accept batches of 1 to 1,000 rows of one kind (sessions, blocks, chunks); the namespace is resolved from the key.
