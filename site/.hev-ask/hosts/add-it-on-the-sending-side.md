---
id: "hosts#add-it-on-the-sending-side"
title: "Hosts"
heading: "Add it on the sending side"
group: "Operate"
order: 168
url: "/docs/hosts#add-it-on-the-sending-side"
anchor: "add-it-on-the-sending-side"
terms: ["sending","side","laptop","host","tables","local","machine","headless","alias","capacity","path","prefixes","optionally","default","check","hosts","target","orders","buildbox","true","prefix","config","dispatch","toml","defaulthost","pathprefix","keep","directories","give","show","diff","before","saving","confirm","reachable","free","slots","shows","version","mismatch"]
hash: "19b869de2889fbda9727e15394fc5e60710479f137e542bff9e3701a98c07f1f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"default_host = \"buildbox\"\n\n[hosts.laptop]\nlocal = true\ncap = 2\n\n[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8\npath_prefix = [\"~/.local/bin\", \"~/go/bin\"]","chunkId":"hosts#add-it-on-the-sending-side"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"hosts#add-it-on-the-sending-side"},{"kind":"code","literal":"hev dispatch hosts","chunkId":"hosts#add-it-on-the-sending-side"},{"kind":"code","literal":"host: buildbox","chunkId":"hosts#add-it-on-the-sending-side"},{"kind":"code","literal":"--host buildbox","chunkId":"hosts#add-it-on-the-sending-side"}]
sources: [{"chunkId":"hosts#add-it-on-the-sending-side","url":"/docs/hosts#add-it-on-the-sending-side","anchor":"add-it-on-the-sending-side"}]
---

On the laptop, add host tables for the local machine and the headless machine (SSH alias, capacity, path prefixes) and optionally a default host, then check the hosts and target orders at the new host.
