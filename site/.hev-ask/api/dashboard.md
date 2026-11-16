---
id: "api/dashboard"
title: "Dashboard API"
heading: null
group: "API"
order: 36
url: "/docs/api/dashboard"
anchor: null
terms: ["introduces","json","behind","local","trace","dashboard","runs","machine","running","server","listens","localhost","default","auth","read","only","http","optional","gzip","unversioned","reads","whichever","archive","keeps","file","search","matches","words","there","grades","while","layer","adds","meaning","based","label","review","routes","belong","hosted"]
hash: "bb9d0d46e7cd4019cb5f4413deb5d51def6e779c8bc4cbbaacd6013d562ad104"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev serve                          # http://127.0.0.1:8787\nhev serve --port 9000\ncurl -s 'http://127.0.0.1:8787/api/sessions?window=7d'","chunkId":"api/dashboard"},{"kind":"code","literal":"hev serve","chunkId":"api/dashboard"},{"kind":"code","literal":"127.0.0.1:8787","chunkId":"api/dashboard"},{"kind":"code","literal":"--port","chunkId":"api/dashboard"},{"kind":"code","literal":"--bind","chunkId":"api/dashboard"},{"kind":"code","literal":"GET","chunkId":"api/dashboard"},{"kind":"code","literal":"HEAD","chunkId":"api/dashboard"},{"kind":"code","literal":"/api/image/{sha256}","chunkId":"api/dashboard"},{"kind":"code","literal":"Content-Type: application/json","chunkId":"api/dashboard"},{"kind":"code","literal":"Accept-Encoding: gzip","chunkId":"api/dashboard"},{"kind":"code","literal":"hev","chunkId":"api/dashboard"},{"kind":"code","literal":"~/.hev/archive","chunkId":"api/dashboard"},{"kind":"code","literal":"[archive] dir","chunkId":"api/dashboard"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"api/dashboard"},{"kind":"code","literal":"GET /","chunkId":"api/dashboard"},{"kind":"code","literal":"GET /ui/","chunkId":"api/dashboard"},{"kind":"code","literal":"/review","chunkId":"api/dashboard"},{"kind":"code","literal":"/api/review/…","chunkId":"api/dashboard"},{"kind":"code","literal":"project","chunkId":"api/dashboard"}]
sources: [{"chunkId":"api/dashboard","url":"/docs/api/dashboard","anchor":null}]
---

Introduces the JSON API behind the local trace dashboard: it runs on the machine running the dashboard server, listens on localhost by default, has no auth, is read-only GET over HTTP with optional gzip, and is unversioned. It reads whichever archive the machine keeps; with the file archive search matches words and there are no grades, while hev layer adds grades and meaning-based search. Label-review routes belong only to hosted dashboards.
