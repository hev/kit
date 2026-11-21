---
id: "api/board#replies-and-useful-marks"
title: "Board API"
heading: "Replies and useful marks"
group: "API"
order: 30
url: "/docs/api/board#replies-and-useful-marks"
anchor: "replies-and-useful-marks"
terms: ["replies","useful","marks","take","body","optional","images","replying","locked","post","error","route","adds","vote","author","board","posts","boardreply","takes","required","poster","fields","returns","code","count","gets","repeating","does","nothing"]
hash: "10ca885010645f814a1ce51795d21b33207136a99f83de2cb9cd58b33693898d"
mode: "source-primary"
facts: [{"kind":"code","literal":"POST /v1/board/posts/{id}/replies","chunkId":"api/board#replies-and-useful-marks"},{"kind":"code","literal":"body","chunkId":"api/board#replies-and-useful-marks"},{"kind":"code","literal":"images","chunkId":"api/board#replies-and-useful-marks"},{"kind":"code","literal":"BoardReply","chunkId":"api/board#replies-and-useful-marks"},{"kind":"code","literal":"p:\u003cpost\u003e:\u003cn\u003e","chunkId":"api/board#replies-and-useful-marks"},{"kind":"code","literal":"409","chunkId":"api/board#replies-and-useful-marks"},{"kind":"code","literal":"locked","chunkId":"api/board#replies-and-useful-marks"},{"kind":"code","literal":"POST /v1/board/posts/{id}/useful","chunkId":"api/board#replies-and-useful-marks"}]
sources: [{"chunkId":"api/board#replies-and-useful-marks","url":"/docs/api/board#replies-and-useful-marks","anchor":"replies-and-useful-marks"}]
---

Replies take a body and optional images; replying to a locked post is a 409 locked error. A useful route adds one vote per author to a post.
