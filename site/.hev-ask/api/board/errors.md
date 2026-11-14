---
id: "api/board#errors"
title: "Board API"
heading: "Errors"
group: "API"
order: 15
url: "/docs/api/board#errors"
anchor: "errors"
terms: ["errors","board","json","object","error","message","fields","writes","read","only","unknown","numeric","thread","oversized","images","unsupported","image","types","invalid","other","faults","layer","store","does","answer","routes","wrong","methods","plain","text","request","needs","title","such","page","found","method","allowed","field","status"]
hash: "e913e4d999b18f61ae0681c26fa655d40f55ef8c8464889b1c1551f564e41bc7"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"error\": \"invalid request: a thread needs a title\"\n}","chunkId":"api/board#errors"},{"kind":"code","literal":"400","chunkId":"api/board#errors"},{"kind":"code","literal":"invalid request:","chunkId":"api/board#errors"},{"kind":"code","literal":"403","chunkId":"api/board#errors"},{"kind":"code","literal":"404","chunkId":"api/board#errors"},{"kind":"code","literal":"no such thread — see hev board ls","chunkId":"api/board#errors"},{"kind":"code","literal":"413","chunkId":"api/board#errors"},{"kind":"code","literal":"415","chunkId":"api/board#errors"},{"kind":"code","literal":"500","chunkId":"api/board#errors"},{"kind":"code","literal":"bad json: …","chunkId":"api/board#errors"},{"kind":"code","literal":"502","chunkId":"api/board#errors"},{"kind":"code","literal":"404 page not found","chunkId":"api/board#errors"},{"kind":"code","literal":"405 Method Not Allowed","chunkId":"api/board#errors"}]
sources: [{"chunkId":"api/board#errors","url":"/docs/api/board#errors","anchor":"errors"}]
---

Board API errors are a JSON object with one error message: 400 for bad fields, 403 for writes to a read-only board, 404 for unknown or non-numeric thread IDs, 413 for oversized images, 415 for unsupported image types, 500 for invalid JSON and other faults, 502 when the hev layer store does not answer. Unknown routes and wrong methods get plain-text 404/405.
