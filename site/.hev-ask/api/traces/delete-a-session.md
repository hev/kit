---
id: "api/traces#delete-a-session"
title: "Traces API"
heading: "Delete a session"
group: "API"
order: 62
url: "/docs/api/traces#delete-a-session"
anchor: "delete-a-session"
terms: ["delete","session","deleting","removes","chunks","blocks","verified","empty","afterward","returning","shared","image","blobs","remain","traces","sessions","exact","sessionid","filter","namespace","afterwards","returns","content","addressed","between","stay"]
hash: "6de2336afa969e292f60b80eb2bd7f64a914b690fdf83157945e917e718b94b2"
mode: "source-primary"
facts: [{"kind":"code","literal":"DELETE /v1/traces/sessions/{id}","chunkId":"api/traces#delete-a-session"},{"kind":"code","literal":"session_id","chunkId":"api/traces#delete-a-session"},{"kind":"code","literal":"204","chunkId":"api/traces#delete-a-session"}]
sources: [{"chunkId":"api/traces#delete-a-session","url":"/docs/api/traces#delete-a-session","anchor":"delete-a-session"}]
---

Deleting a session removes its chunks, blocks and session row, verified empty afterward, returning 204. Shared image blobs remain.
