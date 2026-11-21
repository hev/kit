---
id: "evidence#read-verify-and-search"
title: "Evidence"
heading: "Read, verify and search"
group: "Use"
order: 207
url: "/docs/evidence#read-verify-and-search"
anchor: "read-verify-and-search"
terms: ["read","verify","search","commands","list","show","evidence","filters","order","source","verdict","since","verifies","every","item","hash","size","refuses","damaged","packages","keyword","matching","semantic","finds","images","video","through","captions","names","name","pass","fail","json","query","also","phrasing","sha256","manifest","against","prints"]
hash: "bbb1f4130dc78e3077ef59db299e2d6bc2e16b690789f544fc8b53bbfbb2e799"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev evidence ls [--order NAME] [--source S] [--verdict pass|fail|...] [--tag T] [--since 7d] [--json]\nhev evidence show ID [--json]\nhev evidence get ID [--out DIR]\nhev evidence search QUERY [--also PHRASING]... [the ls filters] [--json]","chunkId":"evidence#read-verify-and-search"},{"kind":"code","literal":"get","chunkId":"evidence#read-verify-and-search"},{"kind":"code","literal":"sha256","chunkId":"evidence#read-verify-and-search"},{"kind":"code","literal":"manifest.json","chunkId":"evidence#read-verify-and-search"},{"kind":"code","literal":"--out DIR","chunkId":"evidence#read-verify-and-search"},{"kind":"code","literal":"search","chunkId":"evidence#read-verify-and-search"},{"kind":"code","literal":"--also","chunkId":"evidence#read-verify-and-search"},{"kind":"code","literal":"--json","chunkId":"evidence#read-verify-and-search"}]
sources: [{"chunkId":"evidence#read-verify-and-search","url":"/docs/evidence#read-verify-and-search","anchor":"read-verify-and-search"}]
---

Commands to list, show, get and search evidence with filters (order, source, verdict, tag, since). get verifies every item's hash and size and refuses damaged packages. Search is keyword matching, not semantic, and finds images and video through captions and names.
