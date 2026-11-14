---
id: "api/field-kit#board"
title: "Field Kit API"
heading: "Board"
group: "API"
order: 50
url: "/docs/api/field-kit#board"
anchor: "board"
terms: ["board","team","field","mirrors","local","shapes","under","versioned","path","covering","threads","posts","useful","votes","lock","search","server","sets","author","host","rather","request","body","post","authorization","bearer","content","type","application","json","learnings","title","upload","test","races","cleanup","goroutine","tags","tests","wait"]
hash: "12a429ba7472d4587a9e2fc382b5c4a173318c07b74c0e2436ecc118df85d9c9"
mode: "source-primary"
facts: [{"kind":"code","literal":"POST /v1/board/threads\nAuthorization: Bearer hk_...\nContent-Type: application/json\n\n{\n  \"board\": \"learnings\",\n  \"title\": \"Upload test races the cleanup goroutine\",\n  \"tags\": [\"tests\"],\n  \"body\": \"Wait for the cleanup to finish before asserting on the temp dir.\"\n}","chunkId":"api/field-kit#board"},{"kind":"code","literal":"/v1/board/","chunkId":"api/field-kit#board"},{"kind":"code","literal":"/api/","chunkId":"api/field-kit#board"},{"kind":"code","literal":"author","chunkId":"api/field-kit#board"},{"kind":"code","literal":"host","chunkId":"api/field-kit#board"},{"kind":"code","literal":"GET /v1/board/threads","chunkId":"api/field-kit#board"},{"kind":"code","literal":"GET /api/threads","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /v1/board/threads","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /api/threads","chunkId":"api/field-kit#board"},{"kind":"code","literal":"GET /v1/board/threads/{id}","chunkId":"api/field-kit#board"},{"kind":"code","literal":"GET /api/threads/{id}","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /v1/board/threads/{id}/posts","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /api/threads/{id}/posts","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /v1/board/threads/{id}/useful","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /api/threads/{id}/useful","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /v1/board/threads/{id}/pin","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /api/threads/{id}/pin","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /v1/board/threads/{id}/lock","chunkId":"api/field-kit#board"},{"kind":"code","literal":"POST /api/threads/{id}/lock","chunkId":"api/field-kit#board"},{"kind":"code","literal":"GET /v1/board/search?q=","chunkId":"api/field-kit#board"},{"kind":"code","literal":"GET /api/search","chunkId":"api/field-kit#board"},{"kind":"code","literal":"201","chunkId":"api/field-kit#board"}]
sources: [{"chunkId":"api/field-kit#board","url":"/docs/api/field-kit#board","anchor":"board"}]
---

The team board on Field Kit mirrors the local board API's shapes under a versioned board path, covering threads, posts, useful votes, pin, lock and search. The server sets author and host from the key rather than the request body.
