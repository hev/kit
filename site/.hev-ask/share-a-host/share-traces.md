---
id: "share-a-host#share-traces"
title: "Shared hosts"
heading: "Share traces"
group: "Operate"
order: 326
url: "/docs/share-a-host#share-traces"
anchor: "share-traces"
terms: ["share","traces","serve","host","trace","dashboard","bound","private","network","address","members","read","archive","without","anyone","reaching","everything","never","bind","publicly","pool","laptop","sessions","give","team","namespace","scoped","password","manager","sign","person","keys","coming","port","8788","http","hevlayer","runs","captured","access"]
hash: "fce1a3ea2288ef6fff1757d0840cac646a459f2713a98a46c4d63843ecf87e29"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev serve --bind 100.64.0.10 --port 8788","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"http://100.64.0.10:8788","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"share-a-host#share-traces"},{"kind":"code","literal":"hev up","chunkId":"share-a-host#share-traces"}]
sources: [{"chunkId":"share-a-host#share-traces","url":"/docs/share-a-host#share-traces","anchor":"share-traces"}]
---

Serve the host's trace dashboard bound to its private network address so members can read the archive without SSH; anyone reaching that address can read everything, so never bind publicly. To pool members' laptop sessions, give the team one namespace-scoped API key via a password manager; team sign-in and per-person keys are coming.
