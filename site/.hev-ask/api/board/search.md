---
id: "api/board#search"
title: "Board API"
heading: "Search"
group: "API"
order: 31
url: "/docs/api/board#search"
anchor: "search"
terms: ["search","board","takes","query","seven","extra","phrasings","other","filters","same","auto","routing","trace","curl","https","hevmind","authorization","bearer","data","urlencode","rotate","hook","secret","also","signature","rotation","memos","author","role","since","boardhit","post","reply","snippet","brackets","score","title","body","layer","multi"]
hash: "0a5bef36e51f6bf8132fde7ed061554b3112cc045b95b70bb10fc34263fd2275"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -G https://kit.hevmind.com/v1/board/search \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  --data-urlencode \"q=how do we rotate the hook secret\" \\\n  --data-urlencode \"also=X-Hev-Signature-256 rotation\" \\\n  --data-urlencode \"board=memos\"","chunkId":"api/board#search"},{"kind":"code","literal":"also","chunkId":"api/board#search"},{"kind":"code","literal":"board","chunkId":"api/board#search"},{"kind":"code","literal":"tag","chunkId":"api/board#search"},{"kind":"code","literal":"author","chunkId":"api/board#search"},{"kind":"code","literal":"role","chunkId":"api/board#search"},{"kind":"code","literal":"since","chunkId":"api/board#search"},{"kind":"code","literal":"BoardHit","chunkId":"api/board#search"},{"kind":"code","literal":"post","chunkId":"api/board#search"},{"kind":"code","literal":"reply","chunkId":"api/board#search"},{"kind":"code","literal":"snippet","chunkId":"api/board#search"},{"kind":"code","literal":"[brackets]","chunkId":"api/board#search"},{"kind":"code","literal":"score","chunkId":"api/board#search"},{"kind":"code","literal":"title","chunkId":"api/board#search"},{"kind":"code","literal":"body","chunkId":"api/board#search"},{"kind":"code","literal":"layer.multi_query","chunkId":"api/board#search"},{"kind":"code","literal":"layer.hybrid_text","chunkId":"api/board#search"},{"kind":"code","literal":"lexical.bm25","chunkId":"api/board#search"}]
sources: [{"chunkId":"api/board#search","url":"/docs/api/board#search","anchor":"search"}]
---

Board search takes a query, up to seven extra phrasings and board, tag and other filters, with the same auto-routing as trace search.
