---
id: "share-a-host#share-traces"
title: "Shared hosts"
heading: "Share traces"
group: "Operate"
order: 282
url: "/docs/share-a-host#share-traces"
anchor: "share-traces"
terms: ["share","traces","members","read","host","trace","archive","without","serve","dashboard","bound","private","network","address","only","server","login","anyone","reach","reads","everything","never","bind","public","interface","port","8788","http","hevlayer","namespace","runs","captured","access","team","machine","example","tailscale","such","tell","stop"]
hash: "e6918cf0b1b0de5f897267e5f8ec53a5d41e66a140916e98354e0fee0bc395ff"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev serve --bind 100.64.0.10 --port 8788","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"http://100.64.0.10:8788","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"hev up","chunkId":"share-a-host#share-traces"}]
sources: [{"chunkId":"share-a-host#share-traces","url":"/docs/share-a-host#share-traces","anchor":"share-traces"}]
---

To let members read the host's trace archive without SSH, serve the dashboard with hev serve bound to the host's private network address only. The server has no login, so anyone who can reach the address reads everything in the archive. Never bind a public interface.
