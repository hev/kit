---
id: "hosts#add-it-on-the-sending-side"
title: "Hosts"
heading: "Add it on the sending side"
group: "Operate"
order: 175
url: "/docs/hosts#add-it-on-the-sending-side"
anchor: "add-it-on-the-sending-side"
terms: ["sending","side","laptop","headless","machine","dispatch","config","host","destination","capacity","path","prefix","directories","optionally","making","default","verify","inventory","send","orders","name","override","order","buildbox","hosts","local","true","toml","defaulthost","pathprefix","keep","alias","give","show","diff","before","saving","confirm","reachable","free"]
hash: "19b869de2889fbda9727e15394fc5e60710479f137e542bff9e3701a98c07f1f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"default_host = \"buildbox\"\n\n[hosts.laptop]\nlocal = true\ncap = 2\n\n[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8\npath_prefix = [\"~/.local/bin\", \"~/go/bin\"]","chunkId":"hosts#add-it-on-the-sending-side"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"hosts#add-it-on-the-sending-side"},{"kind":"code","literal":"hev dispatch hosts","chunkId":"hosts#add-it-on-the-sending-side"},{"kind":"code","literal":"host: buildbox","chunkId":"hosts#add-it-on-the-sending-side"},{"kind":"code","literal":"--host buildbox","chunkId":"hosts#add-it-on-the-sending-side"}]
sources: [{"chunkId":"hosts#add-it-on-the-sending-side","url":"/docs/hosts#add-it-on-the-sending-side","anchor":"add-it-on-the-sending-side"}]
---

On the laptop, add the headless machine to the dispatch config as a host with its SSH destination, capacity and any PATH prefix directories, optionally making it the default host. Then verify with the host inventory and send orders that name it or override the host per order.
