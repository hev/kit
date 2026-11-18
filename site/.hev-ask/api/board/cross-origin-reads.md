---
id: "api/board#cross-origin-reads"
title: "Board API"
heading: "Cross-origin reads"
group: "API"
order: 14
url: "/docs/api/board#cross-origin-reads"
anchor: "cross-origin-reads"
terms: ["cross","origin","reads","board","server","sends","cors","headers","default","listing","config","lets","browser","page","make","read","requests","writes","never","allowed","https","dash","example","options","access","control","allow","another","list","under","comma","separated","such","only","paths","listed"]
hash: "c94c5c6dcff520a79cecec5b7088b08df316297f1e8ab46278cea58337bd40ff"
mode: "source-primary"
facts: [{"kind":"code","literal":"cors","chunkId":"api/board#cross-origin-reads"},{"kind":"code","literal":"[board]","chunkId":"api/board#cross-origin-reads"},{"kind":"code","literal":"cors = \"https://dash.example.com\"","chunkId":"api/board#cross-origin-reads"},{"kind":"code","literal":"GET","chunkId":"api/board#cross-origin-reads"},{"kind":"code","literal":"OPTIONS","chunkId":"api/board#cross-origin-reads"},{"kind":"code","literal":"/api…","chunkId":"api/board#cross-origin-reads"},{"kind":"code","literal":"Access-Control-Allow-Origin","chunkId":"api/board#cross-origin-reads"}]
sources: [{"chunkId":"api/board#cross-origin-reads","url":"/docs/api/board#cross-origin-reads","anchor":"cross-origin-reads"}]
---

The board server sends no CORS headers by default. Listing an origin in the board config lets a browser page on that origin make read requests to the API; writes are never allowed cross-origin.
