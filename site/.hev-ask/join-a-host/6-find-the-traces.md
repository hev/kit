---
id: "join-a-host#6-find-the-traces"
title: "Join a host"
heading: "6. Find the traces"
group: "Operate"
order: 203
url: "/docs/join-a-host#6-find-the-traces"
anchor: "6-find-the-traces"
terms: ["find","traces","runs","shared","host","land","trace","archive","browsable","through","dashboard","address","owner","shares","private","network","filterable","name","also","send","laptop","sessions","team","namespace","setup","command","install","daemon","windows","aware","capture","takes","every","agent","session","machine","scrubbing","imperfect","export","hevlayer"]
hash: "e5df6a6de0268287692d89c40596449d774efee755dbe8679b7027fc58f15d76"
mode: "agent-primary"
facts: [{"kind":"code","literal":"export HEVLAYER_API_KEY=KEY\nexport HEVLAYER_NAMESPACE=NAMESPACE\nhev up\nhev s","chunkId":"join-a-host#6-find-the-traces"},{"kind":"code","literal":"$env:HEVLAYER_API_KEY = \"KEY\"\n$env:HEVLAYER_NAMESPACE = \"NAMESPACE\"\nhev.exe up\nhev.exe daemon install\nhev.exe s","chunkId":"join-a-host#6-find-the-traces"},{"kind":"code","literal":"first-run","chunkId":"join-a-host#6-find-the-traces"}]
sources: [{"chunkId":"join-a-host#6-find-the-traces","url":"/docs/join-a-host#6-find-the-traces","anchor":"6-find-the-traces"}]
---

Runs on the shared host land in its trace archive, browsable through the dashboard address the owner shares on the private network and filterable by your name. To also send your laptop's sessions to the team archive, set the team API key and namespace for one setup command (and install the daemon on Windows); be aware capture takes every agent session on the machine and scrubbing is imperfect.
