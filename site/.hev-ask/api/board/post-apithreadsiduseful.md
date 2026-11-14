---
id: "api/board#post-apithreadsiduseful"
title: "Board API"
heading: "POST /api/threads/\\{id\\}/useful"
group: "API"
order: 34
url: "/docs/api/board#post-apithreadsiduseful"
anchor: "post-apithreadsiduseful"
terms: ["post","threads","useful","marks","thread","behalf","author","counts","once","repeat","votes","change","nothing","missing","curl","http","7700","content","type","application","json","adam","invalid","request","vote","needs","marking","again","changes","returns","same","count","body","field","required","voting"]
hash: "bd996155820eb5e6e6586e05a906dcb8efb2c4ad5eae7b01544b549cbcf15ce6"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -X POST http://127.0.0.1:7700/api/threads/1/useful \\\n  -H \"Content-Type: application/json\" -d '{\"author\":\"adam\"}'","chunkId":"api/board#post-apithreadsiduseful"},{"kind":"code","literal":"{\n  \"thread\": 1,\n  \"useful\": 1\n}","chunkId":"api/board#post-apithreadsiduseful"},{"kind":"code","literal":"author","chunkId":"api/board#post-apithreadsiduseful"},{"kind":"code","literal":"400","chunkId":"api/board#post-apithreadsiduseful"},{"kind":"code","literal":"invalid request: a vote needs an author","chunkId":"api/board#post-apithreadsiduseful"}]
sources: [{"chunkId":"api/board#post-apithreadsiduseful","url":"/docs/api/board#post-apithreadsiduseful","anchor":"post-apithreadsiduseful"}]
---

Marks a thread useful on behalf of an author; each author counts once and repeat votes change nothing. A missing author is 400.
