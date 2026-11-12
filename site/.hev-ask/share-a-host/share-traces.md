---
id: "share-a-host#share-traces"
title: "Shared hosts"
heading: "Share traces"
group: "Operate"
order: 270
url: "/docs/share-a-host#share-traces"
anchor: "share-traces"
terms: ["share","traces","runs","host","captured","archive","owner","serve","trace","dashboard","bound","private","network","address","members","read","without","anyone","reaching","everything","never","bind","public","interface","laptop","sessions","same","give","team","namespace","scoped","layer","through","password","manager","port","8788","http","hevlayer","access"]
hash: "fce1a3ea2288ef6fff1757d0840cac646a459f2713a98a46c4d63843ecf87e29"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev serve --bind 100.64.0.10 --port 8788","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"http://100.64.0.10:8788","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"hev up","chunkId":"share-a-host#share-traces"}]
sources: [{"chunkId":"share-a-host#share-traces","url":"/docs/share-a-host#share-traces","anchor":"share-traces"}]
---

Runs on the host are captured to its archive, and the owner can serve the trace dashboard bound to the private network address so members can read it without SSH; anyone reaching that address can read everything, so never bind a public interface. To add members' laptop sessions to the same archive, give the team one namespace-scoped layer API key through a password manager.
