---
id: "admission#set-the-list"
title: "Admission"
heading: "Set the list"
group: "Operate"
order: 3
url: "/docs/admission#set-the-list"
anchor: "set-the-list"
terms: ["list","level","admission","table","receiving","host","dispatch","config","listing","allowed","repo","origin","urls","https","forms","empty","admits","none","missing","everything","includes","agent","ready","prompt","apply","capacity","repos","github","acme","hosts","buildbox","local","true","toml","path","target","receives","dispatched","work","limit"]
hash: "25152040c651efba423190e1058fa080cc07d1e96f1e6f9f33eead63ac52d1f3"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap = 4 # receiving host capacity\n\n[admission]\nrepos = [\n  \"https://github.com/acme/api.git\",\n  \"git@github.com:acme/web\",\n]\n\n[hosts.buildbox]\nlocal = true\ncap = 4","chunkId":"admission#set-the-list"},{"kind":"code","literal":"[admission]","chunkId":"admission#set-the-list"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"admission#set-the-list"},{"kind":"code","literal":"hev dispatch rpc","chunkId":"admission#set-the-list"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"admission#set-the-list"},{"kind":"code","literal":"--config","chunkId":"admission#set-the-list"},{"kind":"code","literal":"config_path","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos = []","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos = [...]","chunkId":"admission#set-the-list"},{"kind":"code","literal":"target admission config: ...","chunkId":"admission#set-the-list"},{"kind":"code","literal":"cap","chunkId":"admission#set-the-list"}]
sources: [{"chunkId":"admission#set-the-list","url":"/docs/admission#set-the-list","anchor":"set-the-list"}]
---

Add a top-level admission table to the receiving host's dispatch config listing allowed repo origin URLs (HTTPS or SSH forms). An empty list admits none, and a missing list admits everything. Includes an agent-ready prompt to apply this on a host.
