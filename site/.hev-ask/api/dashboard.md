---
id: "api/dashboard"
title: "Dashboard API"
heading: null
group: "API"
order: 36
url: "/docs/api/dashboard"
anchor: null
terms: ["trace","dashboard","server","exposes","local","unauthenticated","unversioned","json","only","optionally","gzipped","localhost","port","8787","default","readable","anyone","reach","reads","file","archive","layer","files","search","matches","words","there","grades","covers","sessions","single","session","stats","filter","values","images","asks","factory","accounting","serve"]
hash: "bb9d0d46e7cd4019cb5f4413deb5d51def6e779c8bc4cbbaacd6013d562ad104"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev serve                          # http://127.0.0.1:8787\nhev serve --port 9000\ncurl -s 'http://127.0.0.1:8787/api/sessions?window=7d'","chunkId":"api/dashboard"},{"kind":"code","literal":"hev serve","chunkId":"api/dashboard"},{"kind":"code","literal":"127.0.0.1:8787","chunkId":"api/dashboard"},{"kind":"code","literal":"--port","chunkId":"api/dashboard"},{"kind":"code","literal":"--bind","chunkId":"api/dashboard"},{"kind":"code","literal":"GET","chunkId":"api/dashboard"},{"kind":"code","literal":"HEAD","chunkId":"api/dashboard"},{"kind":"code","literal":"/api/image/{sha256}","chunkId":"api/dashboard"},{"kind":"code","literal":"Content-Type: application/json","chunkId":"api/dashboard"},{"kind":"code","literal":"Accept-Encoding: gzip","chunkId":"api/dashboard"},{"kind":"code","literal":"hev","chunkId":"api/dashboard"},{"kind":"code","literal":"~/.hev/archive","chunkId":"api/dashboard"},{"kind":"code","literal":"[archive] dir","chunkId":"api/dashboard"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"api/dashboard"},{"kind":"code","literal":"GET /","chunkId":"api/dashboard"},{"kind":"code","literal":"GET /ui/","chunkId":"api/dashboard"},{"kind":"code","literal":"/review","chunkId":"api/dashboard"},{"kind":"code","literal":"/api/review/…","chunkId":"api/dashboard"},{"kind":"code","literal":"project","chunkId":"api/dashboard"}]
sources: [{"chunkId":"api/dashboard","url":"/docs/api/dashboard","anchor":null}]
---

The trace dashboard server exposes a local, unauthenticated, unversioned JSON API (GET only, optionally gzipped) on localhost port 8787 by default, readable by anyone who can reach the port. It reads the file archive or hev layer; with files, search matches words only and there are no grades. It covers sessions, a single session, stats, search, filter values, images, asks and factory accounting.
