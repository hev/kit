---
id: "clustering#add-it-on-the-sending-side"
title: "Clustering"
heading: "Add it on the sending side"
group: "Operate"
order: 112
url: "/docs/clustering#add-it-on-the-sending-side"
anchor: "add-it-on-the-sending-side"
terms: ["sending","side","sender","host","table","dispatch","config","target","capacity","optional","path","prefix","default","includes","agent","prompt","buildbox","hosts","laptop","local","true","toml","defaulthost","pathprefix","headless","machine","keep","alias","directories","give","show","diff","before","saving","confirm","reachable","free","slots","shows","version"]
hash: "c51fc37d09a24a541b3f410fbba5be00035bce792711101abf55719e532c859c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"default_host = \"buildbox\"\n\n[hosts.laptop]\nlocal = true\ncap = 2\n\n[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8\npath_prefix = [\"~/.local/bin\", \"~/go/bin\"]","chunkId":"clustering#add-it-on-the-sending-side"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"clustering#add-it-on-the-sending-side"},{"kind":"code","literal":"hev dispatch hosts","chunkId":"clustering#add-it-on-the-sending-side"},{"kind":"code","literal":"host: buildbox","chunkId":"clustering#add-it-on-the-sending-side"},{"kind":"code","literal":"--host buildbox","chunkId":"clustering#add-it-on-the-sending-side"}]
sources: [{"chunkId":"clustering#add-it-on-the-sending-side","url":"/docs/clustering#add-it-on-the-sending-side","anchor":"add-it-on-the-sending-side"}]
---

On the sender, add a host table in the dispatch config with ssh target, capacity and optional path prefix, and set a default host. Includes an agent prompt.
