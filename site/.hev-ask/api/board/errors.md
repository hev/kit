---
id: "api/board#errors"
title: "Board API"
heading: "Errors"
group: "API"
order: 15
url: "/docs/api/board#errors"
anchor: "errors"
terms: ["errors","board","json","object","single","message","field","table","maps","statuses","causes","fields","writes","read","only","unknown","threads","oversized","unsupported","image","uploads","invalid","other","server","faults","store","answer","kept","layer","routes","wrong","methods","return","plain","text","error","request","thread","needs","title"]
hash: "e913e4d999b18f61ae0681c26fa655d40f55ef8c8464889b1c1551f564e41bc7"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"error\": \"invalid request: a thread needs a title\"\n}","chunkId":"api/board#errors"},{"kind":"code","literal":"400","chunkId":"api/board#errors"},{"kind":"code","literal":"invalid request:","chunkId":"api/board#errors"},{"kind":"code","literal":"403","chunkId":"api/board#errors"},{"kind":"code","literal":"404","chunkId":"api/board#errors"},{"kind":"code","literal":"no such thread — see hev board ls","chunkId":"api/board#errors"},{"kind":"code","literal":"413","chunkId":"api/board#errors"},{"kind":"code","literal":"415","chunkId":"api/board#errors"},{"kind":"code","literal":"500","chunkId":"api/board#errors"},{"kind":"code","literal":"bad json: …","chunkId":"api/board#errors"},{"kind":"code","literal":"502","chunkId":"api/board#errors"},{"kind":"code","literal":"404 page not found","chunkId":"api/board#errors"},{"kind":"code","literal":"405 Method Not Allowed","chunkId":"api/board#errors"}]
sources: [{"chunkId":"api/board#errors","url":"/docs/api/board#errors","anchor":"errors"}]
---

Board API errors are a JSON object with a single message field. The table maps statuses to causes: bad fields, writes to a read-only board, unknown threads, oversized or unsupported image uploads, invalid JSON and other server faults, and a store that did not answer when the board is kept in hev layer; unknown routes and wrong methods return plain text.
