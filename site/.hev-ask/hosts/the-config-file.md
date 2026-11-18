---
id: "hosts#the-config-file"
title: "Hosts"
heading: "The config file"
group: "Operate"
order: 183
url: "/docs/hosts#the-config-file"
anchor: "the-config-file"
terms: ["config","file","dispatch","read","explicit","flag","environment","variable","default","path","same","format","senders","targets","send","only","laptop","needs","just","host","entries","while","receiving","reads","copy","admission","notifications","launch","settings","shows","example","windows","local","hosts","unknown","keys","error","toml","first","these"]
hash: "8b7f408c96452178f214a9248ac3373ab468c6862a45a34718eddb68588639fa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--config PATH","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"hev dispatch","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"~/","chunkId":"hosts#the-config-file"}]
sources: [{"chunkId":"hosts#the-config-file","url":"/docs/hosts#the-config-file","anchor":"the-config-file"}]
---

The dispatch config is read from an explicit flag, then an environment variable, then the default path, and the same format is used on senders and targets. A send-only laptop needs just host entries, while a receiving host reads its own copy for admission, notifications and launch settings. Shows an example with SSH, Windows and local hosts; unknown keys are an error.
