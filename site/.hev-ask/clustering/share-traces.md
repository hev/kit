---
id: "clustering#share-traces"
title: "Clustering"
heading: "Share traces"
group: "Operate"
order: 128
url: "/docs/clustering#share-traces"
anchor: "share-traces"
terms: ["share","traces","members","read","host","trace","archive","without","serve","dashboard","bound","only","private","network","address","bind","port","8788","http","hevlayer","namespace","runs","captured","access","team","machine","example","tailscale","such","never","public","interface","tell","stop","verify","answers","give","docs","https","hevkit"]
hash: "623d37ebf24f21f958fe321b51d676936d83d806a93fad1436d072ea30036975"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev serve --bind 100.64.0.10 --port 8788","chunkId":"clustering#share-traces"},{"kind":"code","literal":"http://100.64.0.10:8788","chunkId":"clustering#share-traces"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"clustering#share-traces"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"clustering#share-traces"},{"kind":"code","literal":"hev up","chunkId":"clustering#share-traces"}]
sources: [{"chunkId":"clustering#share-traces","url":"/docs/clustering#share-traces","anchor":"share-traces"}]
---

To let members read the host's trace archive without SSH, serve the dashboard bound only to the private network address.
