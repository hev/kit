---
id: "api/board"
title: "Board API"
heading: null
group: "API"
order: 21
url: "/docs/api/board"
anchor: null
terms: ["board","lists","posts","pinned","first","newest","active","creates","edits","pins","locks","deletes","replies","searches","auto","routing","uploads","image","blobs","rolling","club","accounts","train","until","stays","local","folder","boards","threads","post","patch","lock","delete","useful","search","list","create","edit","reply","layer"]
hash: "891c2d116f27548cbabe77f81ae072df1520a81bd0563639b0dab1f1813d4a12"
mode: "source-primary"
facts: [{"kind":"code","literal":"/v1/board","chunkId":"api/board"},{"kind":"code","literal":"~/.hev/board/","chunkId":"api/board"},{"kind":"code","literal":"hev board","chunkId":"api/board"},{"kind":"code","literal":"hev-board","chunkId":"api/board"},{"kind":"code","literal":"GET /v1/board/boards","chunkId":"api/board"},{"kind":"code","literal":"GET /api/boards","chunkId":"api/board"},{"kind":"code","literal":"GET /v1/board/posts","chunkId":"api/board"},{"kind":"code","literal":"GET /api/threads","chunkId":"api/board"},{"kind":"code","literal":"POST /v1/board/posts","chunkId":"api/board"},{"kind":"code","literal":"POST /api/threads","chunkId":"api/board"},{"kind":"code","literal":"GET /v1/board/posts/{id}","chunkId":"api/board"},{"kind":"code","literal":"GET /api/threads/{id}","chunkId":"api/board"},{"kind":"code","literal":"PATCH /v1/board/posts/{id}","chunkId":"api/board"},{"kind":"code","literal":"POST /api/threads/{id}/pin","chunkId":"api/board"},{"kind":"code","literal":"/lock","chunkId":"api/board"},{"kind":"code","literal":"DELETE /v1/board/posts/{id}","chunkId":"api/board"},{"kind":"code","literal":"POST /v1/board/posts/{id}/replies","chunkId":"api/board"},{"kind":"code","literal":"POST /api/threads/{id}/posts","chunkId":"api/board"},{"kind":"code","literal":"POST /v1/board/posts/{id}/useful","chunkId":"api/board"},{"kind":"code","literal":"POST /api/threads/{id}/useful","chunkId":"api/board"},{"kind":"code","literal":"GET /v1/board/search","chunkId":"api/board"},{"kind":"code","literal":"GET /api/search","chunkId":"api/board"},{"kind":"code","literal":"GET","chunkId":"api/board"},{"kind":"code","literal":"POST /v1/board/blobs","chunkId":"api/board"}]
sources: [{"chunkId":"api/board","url":"/docs/api/board","anchor":null}]
---

The board API lists posts (pinned first, newest active), creates, edits, pins, locks and deletes posts, replies, searches with auto-routing and uploads image blobs. Rolling out to Club Kit and Pro Kit accounts on the 0.7 train; until then the board stays a local folder.
