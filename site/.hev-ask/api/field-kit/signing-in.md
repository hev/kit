---
id: "api/field-kit#signing-in"
title: "Field Kit API"
heading: "Signing in"
group: "API"
order: 62
url: "/docs/api/field-kit#signing-in"
anchor: "signing-in"
terms: ["signing","uses","browser","time","code","pkce","starts","local","callback","listener","opens","login","page","user","signs","picks","team","redirects","back","exchanges","mismatched","state","rejected","challenge","port","verifier","http","post","exchange","error","invalid","machine","need","call","these","routes","yourself","here","know","does"]
hash: "000698ea114992d7914f2e48f4483911d390035e6f4fc11995675e1d45af9d81"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev login","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"127.0.0.1","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"GET /cli/login?state=STATE\u0026challenge=CHALLENGE\u0026port=PORT","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"state","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"challenge","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"verifier","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"port","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"http://127.0.0.1:PORT/callback","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"code","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"POST /cli/exchange","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"/cli/login","chunkId":"api/field-kit#signing-in"},{"kind":"code","literal":"400 {\"error\":\"invalid_login\"}","chunkId":"api/field-kit#signing-in"},{"kind":"value","literal":"SHA-256","chunkId":"api/field-kit#signing-in"}]
sources: [{"chunkId":"api/field-kit#signing-in","url":"/docs/api/field-kit#signing-in","anchor":"signing-in"}]
---

Signing in uses a browser, a one-time code and PKCE: the CLI starts a local callback listener and opens the login page, the user signs in and picks a team, the browser redirects back with a code, and the CLI exchanges it for a kit key. Mismatched state is rejected.
