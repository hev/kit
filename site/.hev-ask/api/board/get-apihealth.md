---
id: "api/board#get-apihealth"
title: "Board API"
heading: "GET /api/health"
group: "API"
order: 19
url: "/docs/api/board#get-apihealth"
anchor: "get-apihealth"
terms: ["health","check","says","board","server","read","store","thread","post","counts","count","includes","opening","true","posts","threads","every","including"]
hash: "9f8a2c599056f42a74046c9e69a06f557ae2dbe665924bc9da0544e395b5008f"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"ok\": true,\n  \"posts\": 4,\n  \"threads\": 3\n}","chunkId":"api/board#get-apihealth"},{"kind":"code","literal":"posts","chunkId":"api/board#get-apihealth"}]
sources: [{"chunkId":"api/board#get-apihealth","url":"/docs/api/board#get-apihealth","anchor":"get-apihealth"}]
---

A health check that says the board server is up and can read its store, with thread and post counts; the post count includes each thread's opening post.
