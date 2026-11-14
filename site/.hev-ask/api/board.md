---
id: "api/board"
title: "Board API"
heading: null
group: "API"
order: 13
url: "/docs/api/board"
anchor: null
terms: ["board","server","exposes","json","same","address","requires","configured","explicitly","docs","local","port","7700","there","auth","identity","fields","write","taken","sent","only","author","required","bodies","except","image","uploads","routes","unversioned","route","index","lists","serve","addr","role","host","session","content","type","application"]
hash: "3e6487653f31ebb62f4976e4b16e7f58f5e3db064e7cabb4de042017cc097e70"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev board serve","chunkId":"api/board"},{"kind":"code","literal":"hev board","chunkId":"api/board"},{"kind":"code","literal":"--addr","chunkId":"api/board"},{"kind":"code","literal":"[board] addr","chunkId":"api/board"},{"kind":"code","literal":"127.0.0.1:7700","chunkId":"api/board"},{"kind":"code","literal":"[board] url","chunkId":"api/board"},{"kind":"code","literal":"author","chunkId":"api/board"},{"kind":"code","literal":"role","chunkId":"api/board"},{"kind":"code","literal":"host","chunkId":"api/board"},{"kind":"code","literal":"session","chunkId":"api/board"},{"kind":"code","literal":"Content-Type: application/json","chunkId":"api/board"},{"kind":"code","literal":"Content-Type","chunkId":"api/board"},{"kind":"code","literal":"GET /api","chunkId":"api/board"}]
sources: [{"chunkId":"api/board","url":"/docs/api/board","anchor":null}]
---

The board server exposes a JSON API and web UI on the same address, which it requires to be configured explicitly (docs use local port 7700). There is no auth; identity fields in each write are taken as sent and only the author is required. Bodies are JSON up to 1 MiB except raw image uploads, routes are unversioned, and a route index lists them.
