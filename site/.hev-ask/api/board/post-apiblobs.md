---
id: "api/board#post-apiblobs"
title: "Board API"
heading: "POST /api/blobs"
group: "API"
order: 28
url: "/docs/api/board#post-apiblobs"
anchor: "post-apiblobs"
terms: ["post","blobs","uploads","image","bytes","server","detects","type","stores","hash","disk","either","storage","mode","returns","repeat","upload","oversized","images","anything","jpeg","webp","including","empty","bodies","returned","uses","attach","curl","http","7700","data","binary","terminal","sha256","b78a28269a30bb868511ae366143b5b3e44ca9cdcdf49567b1e2702681f806c1","size","width","height","blob"]
hash: "64b46ac3b913fef32c707f7c3a923eb9ea9ed6d49661fd917bc8c754e95f01e8"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -X POST http://127.0.0.1:7700/api/blobs --data-binary @terminal.png","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"{\n  \"sha256\": \"b78a28269a30bb868511ae366143b5b3e44ca9cdcdf49567b1e2702681f806c1\",\n  \"type\": \"image/png\",\n  \"size\": 73,\n  \"width\": 4,\n  \"height\": 2,\n  \"url\": \"/blob/b78a28269a30bb868511ae366143b5b3e44ca9cdcdf49567b1e2702681f806c1\",\n  \"created\": true\n}","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"{\n  \"error\": \"image is too large: it is over the 50 bytes limit\"\n}","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"{\n  \"error\": \"not an accepted image: SVG is not accepted (it can carry scripts); export it as PNG\"\n}","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"Content-Type","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"201","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"200","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"\"created\": false","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"sha256","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"images","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"413","chunkId":"api/board#post-apiblobs"},{"kind":"code","literal":"415","chunkId":"api/board#post-apiblobs"},{"kind":"value","literal":"SHA-256","chunkId":"api/board#post-apiblobs"}]
sources: [{"chunkId":"api/board#post-apiblobs","url":"/docs/api/board#post-apiblobs","anchor":"post-apiblobs"}]
---

Uploads one image as raw bytes; the server detects the type from the bytes and stores the image by its hash on its own disk in either storage mode. A new image returns 201 and a repeat upload returns 200; oversized images are 413 and anything that is not PNG, JPEG, GIF or WebP (including SVG and empty bodies) is 415. The returned hash is what a post uses to attach the image.
