---
id: "admission#set-the-list"
title: "Admission"
heading: "Set the list"
group: "Operate"
order: 3
url: "/docs/admission#set-the-list"
anchor: "set-the-list"
terms: ["list","admission","table","receiving","host","default","dispatch","config","state","means","admits","every","repo","empty","none","filled","only","matching","origins","reads","policy","never","sender","chosen","unreadable","configured","file","makes","refuse","work","capacity","also","comes","lower","raise","repos","https","github","acme","hosts"]
hash: "2c1ce81869e9a3289c8f7c2a75247aa1b426525cdf8d96bdf0b33db9aa8ec057"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cap = 4 # receiving host capacity\n\n[admission]\nrepos = [\n  \"https://github.com/acme/api.git\",\n  \"git@github.com:acme/web\",\n]\n\n[hosts.buildbox]\nlocal = true\ncap = 4","chunkId":"admission#set-the-list"},{"kind":"code","literal":"[admission]","chunkId":"admission#set-the-list"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"admission#set-the-list"},{"kind":"code","literal":"hev dispatch rpc","chunkId":"admission#set-the-list"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"admission#set-the-list"},{"kind":"code","literal":"--config","chunkId":"admission#set-the-list"},{"kind":"code","literal":"config_path","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos = []","chunkId":"admission#set-the-list"},{"kind":"code","literal":"repos = [...]","chunkId":"admission#set-the-list"},{"kind":"code","literal":"target admission config: ...","chunkId":"admission#set-the-list"},{"kind":"code","literal":"cap","chunkId":"admission#set-the-list"}]
sources: [{"chunkId":"admission#set-the-list","url":"/docs/admission#set-the-list","anchor":"set-the-list"}]
---

How to add the admission table to the receiving host's default dispatch config, and what each state means: no table or no list admits every repo, an empty list admits none, and a filled list admits only matching origins. The host reads policy only from its default config, never from a sender's chosen config; an unreadable configured file makes the host refuse all work. Capacity also comes from this default config and a sender can lower but not raise it.
