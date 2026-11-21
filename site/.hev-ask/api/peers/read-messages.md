---
id: "api/peers#read-messages"
title: "Peers API"
heading: "Read messages"
group: "API"
order: 57
url: "/docs/api/peers#read-messages"
anchor: "read-messages"
terms: ["read","messages","reads","peer","inbox","oldest","first","sent","newest","filters","pending","unread","chat","thread","peers","json","true","status","unacknowledged","historical","default","does","keeps","current","incarnation","acknowledged","those","consumed","page","carries","inboxstatus","held","earlier","reading","never","consumes","message","mark","receipt"]
hash: "0d83949bbcff8a625574afd7b59859683385ff5341b577e3b187680a97799af6"
mode: "source-primary"
facts: [{"kind":"code","literal":"GET /v1/peers/{id}/messages","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"box=inbox","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"peers inbox --json","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"box=sent","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"pending=true","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"unread=true","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"thread","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"inbox_status","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"unread","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"unacknowledged","chunkId":"api/peers#read-messages"},{"kind":"code","literal":"historical","chunkId":"api/peers#read-messages"}]
sources: [{"chunkId":"api/peers#read-messages","url":"/docs/api/peers#read-messages","anchor":"read-messages"}]
---

Reads a peer's inbox (oldest first) or sent box (newest first), with filters for pending, unread and chat thread.
