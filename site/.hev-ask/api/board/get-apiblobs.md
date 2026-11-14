---
id: "api/board#get-apiblobs"
title: "Board API"
heading: "GET /api/blobs"
group: "API"
order: 17
url: "/docs/api/board#get-apiblobs"
anchor: "get-apiblobs"
terms: ["blobs","reports","image","upload","limits","maximum","size","default","configurable","images","post","accepted","types","jpeg","webp","bytes","10485760","board","maxbytes","maxperpost","limit","unless","maximagebytes","under","hevboardmaximagebytes","sets","another","most","reply","carry"]
hash: "e84ae24761c3153b6229760252f92734ee0ad2e0e11ec08abb5b41968261ec8c"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"max_bytes\": 10485760,\n  \"max_per_post\": 8,\n  \"types\": [\n    \"image/png\",\n    \"image/jpeg\",\n    \"image/gif\",\n    \"image/webp\"\n  ]\n}","chunkId":"api/board#get-apiblobs"},{"kind":"code","literal":"max_bytes","chunkId":"api/board#get-apiblobs"},{"kind":"code","literal":"max_image_bytes","chunkId":"api/board#get-apiblobs"},{"kind":"code","literal":"[board]","chunkId":"api/board#get-apiblobs"},{"kind":"code","literal":"HEV_BOARD_MAX_IMAGE_BYTES","chunkId":"api/board#get-apiblobs"},{"kind":"code","literal":"max_per_post","chunkId":"api/board#get-apiblobs"}]
sources: [{"chunkId":"api/board#get-apiblobs","url":"/docs/api/board#get-apiblobs","anchor":"get-apiblobs"}]
---

Reports image upload limits: the maximum size per image (10 MiB by default, configurable), the maximum images per post (8), and the accepted types PNG, JPEG, GIF and WebP.
