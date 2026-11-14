---
id: "admission#set-the-list"
title: "Admission"
heading: "Set the list"
group: "Operate"
order: 3
url: "/docs/admission#set-the-list"
anchor: "set-the-list"
terms: ["list","restrict","receiving","host","adding","admission","table","origin","urls","default","dispatch","config","environment","variable","path","standard","file","admits","everything","empty","nothing","listed","only","matching","origins","sender","alternate","never","chooses","policy","unreadable","configured","makes","refuse","work","capacity","also","read","repos","https"]
hash: "2c1ce81869e9a3289c8f7c2a75247aa1b426525cdf8d96bdf0b33db9aa8ec057"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap = 4 # receiving host capacity\n\n[admission]\nrepos = [\n  \"https://github.com/acme/api.git\",\n  \"git@github.com:acme/web\",\n]\n\n[hosts.buildbox]\nlocal = true\ncap = 4","chunkId":"admission#set-the-list"},{"kind":"code","literal":"[admission]","chunkId":"admission#set-the-list"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"admission#set-the-list"},{"kind":"code","literal":"hev dispatch rpc","chunkId":"admission#set-the-list"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"admission#set-the-list"},{"kind":"code","literal":"--config","chunkId":"admission#set-the-list"},{"kind":"code","literal":"config_path","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos = []","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos = [...]","chunkId":"admission#set-the-list"},{"kind":"code","literal":"target admission config: ...","chunkId":"admission#set-the-list"},{"kind":"code","literal":"cap","chunkId":"admission#set-the-list"}]
sources: [{"chunkId":"admission#set-the-list","url":"/docs/admission#set-the-list","anchor":"set-the-list"}]
---

Restrict a receiving host by adding an admission table with a list of origin URLs to its default dispatch config (the environment-variable path or the standard dispatch config file). No table or no list admits everything, an empty list admits nothing, and a listed set admits only matching origins. A sender's alternate config never chooses admission policy; an unreadable configured file makes the host refuse all work. Host capacity is also read from this default config.
