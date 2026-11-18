---
id: "api/board#read-only-mode"
title: "Board API"
heading: "Read-only mode"
group: "API"
order: 35
url: "/docs/api/board#read-only-mode"
anchor: "read-only-mode"
terms: ["read","only","mode","board","server","rejects","every","write","request","route","before","reaches","handler","reads","work","usual","except","reading","thread","longer","counts","view","error","writes","disabled","serve","true","head","options","readonly","under","status"]
hash: "1aa8a64093612740fd6b094f0c482c69ff8f34f3e0d5969e19fda429d1e85c81"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"error\": \"board is read-only; writes are disabled\"\n}","chunkId":"api/board#read-only-mode"},{"kind":"code","literal":"hev board serve --read-only","chunkId":"api/board#read-only-mode"},{"kind":"code","literal":"read_only = true","chunkId":"api/board#read-only-mode"},{"kind":"code","literal":"[board]","chunkId":"api/board#read-only-mode"},{"kind":"code","literal":"GET","chunkId":"api/board#read-only-mode"},{"kind":"code","literal":"HEAD","chunkId":"api/board#read-only-mode"},{"kind":"code","literal":"OPTIONS","chunkId":"api/board#read-only-mode"},{"kind":"code","literal":"403","chunkId":"api/board#read-only-mode"}]
sources: [{"chunkId":"api/board#read-only-mode","url":"/docs/api/board#read-only-mode","anchor":"read-only-mode"}]
---

In read-only mode the board server rejects every write request on every route with a 403 before it reaches a handler. Reads work as usual except that reading a thread no longer counts a view.
