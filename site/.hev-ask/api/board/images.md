---
id: "api/board#images"
title: "Board API"
heading: "Images"
group: "API"
order: 26
url: "/docs/api/board#images"
anchor: "images"
terms: ["images","board","upload","bytes","blobs","route","content","addressed","write","once","type","sniffed","jpeg","webp","refused","curl","https","hevmind","authorization","bearer","data","binary","screenshot","post","sha256","size","width","height","created","etag","cache","control","public","31536000","immutable","yaml","hevapikey","takes","answer","already"]
hash: "c3c5788b89894adaeec197c67b8e610422814e729a9ea0c8428cab6a8663ec93"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s https://kit.hevmind.com/v1/board/blobs \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  --data-binary @screenshot.png","chunkId":"api/board#images"},{"kind":"code","literal":"POST /v1/board/blobs","chunkId":"api/board#images"},{"kind":"code","literal":"Content-Type","chunkId":"api/board#images"},{"kind":"code","literal":"415","chunkId":"api/board#images"},{"kind":"code","literal":"201","chunkId":"api/board#images"},{"kind":"code","literal":"200","chunkId":"api/board#images"},{"kind":"code","literal":"sha256","chunkId":"api/board#images"},{"kind":"code","literal":"type","chunkId":"api/board#images"},{"kind":"code","literal":"size","chunkId":"api/board#images"},{"kind":"code","literal":"width","chunkId":"api/board#images"},{"kind":"code","literal":"height","chunkId":"api/board#images"},{"kind":"code","literal":"url","chunkId":"api/board#images"},{"kind":"code","literal":"created","chunkId":"api/board#images"},{"kind":"code","literal":"GET /v1/board/blobs","chunkId":"api/board#images"},{"kind":"code","literal":"max_bytes","chunkId":"api/board#images"},{"kind":"code","literal":"max_per_post","chunkId":"api/board#images"},{"kind":"code","literal":"GET /v1/board/blobs/{sha256}","chunkId":"api/board#images"},{"kind":"code","literal":"ETag","chunkId":"api/board#images"},{"kind":"code","literal":"Cache-Control: public, max-age=31536000, immutable","chunkId":"api/board#images"},{"kind":"value","literal":"kit-v1.yaml","chunkId":"api/board#images"}]
sources: [{"chunkId":"api/board#images","url":"/docs/api/board#images","anchor":"images"}]
---

Board images upload as raw bytes to a blobs route, are content-addressed and write-once, and the type is sniffed (PNG, JPEG, GIF, WebP; SVG refused with 415).
