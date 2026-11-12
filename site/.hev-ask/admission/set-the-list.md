---
id: "admission#set-the-list"
title: "Admission"
heading: "Set the list"
group: "Operate"
order: 3
url: "/docs/admission#set-the-list"
anchor: "set-the-list"
terms: ["list","owner","sets","admitted","repos","level","admission","table","receiving","host","dispatch","config","listing","origin","urls","page","gives","prompt","agent","make","edit","show","diff","first","capacity","https","github","acme","hosts","buildbox","local","true","toml","path","target","receives","dispatched","work","limit","default"]
hash: "2c1ce81869e9a3289c8f7c2a75247aa1b426525cdf8d96bdf0b33db9aa8ec057"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap = 4 # receiving host capacity\n\n[admission]\nrepos = [\n  \"https://github.com/acme/api.git\",\n  \"git@github.com:acme/web\",\n]\n\n[hosts.buildbox]\nlocal = true\ncap = 4","chunkId":"admission#set-the-list"},{"kind":"code","literal":"[admission]","chunkId":"admission#set-the-list"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"admission#set-the-list"},{"kind":"code","literal":"hev dispatch rpc","chunkId":"admission#set-the-list"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"admission#set-the-list"},{"kind":"code","literal":"--config","chunkId":"admission#set-the-list"},{"kind":"code","literal":"config_path","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos = []","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos = [...]","chunkId":"admission#set-the-list"},{"kind":"code","literal":"target admission config: ...","chunkId":"admission#set-the-list"},{"kind":"code","literal":"cap","chunkId":"admission#set-the-list"}]
sources: [{"chunkId":"admission#set-the-list","url":"/docs/admission#set-the-list","anchor":"set-the-list"}]
---

The owner sets the admitted repos with a top-level admission table in the receiving host's dispatch config, listing origin URLs. The page gives a prompt for an agent to make the edit and show a diff first.
