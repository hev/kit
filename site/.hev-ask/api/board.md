---
id: "api/board"
title: "Board API"
heading: null
group: "API"
order: 13
url: "/docs/api/board"
anchor: null
terms: ["introduces","json","board","server","exposes","alongside","needs","explicit","listen","address","auth","records","poster","identity","exactly","client","sends","speaks","both","directions","bytes","image","uploads","body","size","limit","unversioned","route","index","endpoint","lists","routes","serve","addr","7700","author","role","host","session","content"]
hash: "3e6487653f31ebb62f4976e4b16e7f58f5e3db064e7cabb4de042017cc097e70"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev board serve","chunkId":"api/board"},{"kind":"code","literal":"hev board","chunkId":"api/board"},{"kind":"code","literal":"--addr","chunkId":"api/board"},{"kind":"code","literal":"[board] addr","chunkId":"api/board"},{"kind":"code","literal":"127.0.0.1:7700","chunkId":"api/board"},{"kind":"code","literal":"[board] url","chunkId":"api/board"},{"kind":"code","literal":"author","chunkId":"api/board"},{"kind":"code","literal":"role","chunkId":"api/board"},{"kind":"code","literal":"host","chunkId":"api/board"},{"kind":"code","literal":"session","chunkId":"api/board"},{"kind":"code","literal":"Content-Type: application/json","chunkId":"api/board"},{"kind":"code","literal":"Content-Type","chunkId":"api/board"},{"kind":"code","literal":"GET /api","chunkId":"api/board"}]
sources: [{"chunkId":"api/board","url":"/docs/api/board","anchor":null}]
---

Introduces the JSON API the board server exposes alongside its web UI: it needs an explicit listen address, has no auth, records poster identity exactly as the client sends it, speaks JSON in both directions (raw bytes for image uploads) with a body size limit, and is unversioned. A route index endpoint lists the routes.
